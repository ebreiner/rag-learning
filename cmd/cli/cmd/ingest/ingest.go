package ingest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"rag/cmd/cli/cmd"
	chunkstep "rag/internal/chunk/step"
	embedstep "rag/internal/embedding/step"
	extractstep "rag/internal/extract/step"
	"rag/internal/platform/config"
	"rag/internal/platform/extraction/docling"
	"rag/internal/platform/extraction/source"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/chunk"
	"rag/internal/platform/sqlite/embedding"
	"rag/internal/platform/sqlite/extraction"
	"rag/internal/platform/telemetry/logging"
	"time"
)

func ingest(ctx context.Context, logger *slog.Logger, cnf config.Config, rechunk bool) (err error) {
	db, err := sqlite.NewConn(cnf.Storage.DBPath, true)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing db: %w", closeErr))
		}
	}()

	extLogger := logger.With(logging.KeyStep, "extraction")
	extParams := extractionParams{
		Sources:        cnf.Sources,
		DoclingURL:     cnf.Docling.URL,
		DoclingHeader:  cnf.Docling.Headers,
		DumpDir:        cnf.Docling.DumpDir,
		DoclingTimeout: cnf.Docling.Timeout,
	}
	if err := sqlite.SaveSources(ctx, db, cnf.Sources); err != nil {
		return fmt.Errorf("saving sources failed: %w", err)
	}
	if err := runExtractions(ctx, db, extParams, extLogger); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	chunkParams := chunkParams{
		Flush: rechunk,
	}

	chunkLogger := logger.With(logging.KeyStep, "chunk")
	if err := RunChunking(ctx, db, chunkParams, chunkLogger); err != nil {
		return fmt.Errorf("chunking failed: %w", err)
	}

	embedLogger := logger.With(logging.KeyStep, "embed")
	embeddingParams := embedParams{
		Dimensions: cnf.Embedding.Dim,
		Model:      cnf.Embedding.Model,
	}
	embeddingClient, err := cmd.NewEmbedClient(cnf.Embedding, embedLogger)
	if err != nil {
		return fmt.Errorf("error setting up embedding client: %w", err)
	}

	if err := runEmbedding(ctx, db, embeddingClient, embeddingParams, embedLogger); err != nil {
		return fmt.Errorf("embedding failed: %w", err)
	}

	return nil
}

type extractionParams struct {
	Sources        []config.Source
	DoclingURL     *url.URL
	DoclingHeader  http.Header
	DoclingTimeout time.Duration
	DumpDir        string
}

func runExtractions(ctx context.Context, db *sql.DB, params extractionParams, logger *slog.Logger) error {
	sourceDocSource, err := source.NewSourceDocSource(ctx, params.Sources, logger)
	if err != nil {
		return fmt.Errorf("error creating docs source: %w", err)
	}

	extracedDocSink, err := extraction.NewExtractedDocSink(db, logger)
	if err != nil {
		return fmt.Errorf("error creating docs sink: %w", err)
	}

	var client *http.Client
	if len(params.DumpDir) != 0 {
		client, err = httpclient.NewDump(params.DoclingTimeout, params.DumpDir, params.DoclingHeader, logger)
		if err != nil {
			return err
		}
	} else {
		client = httpclient.New(params.DoclingTimeout, params.DoclingHeader)
	}

	extractor, err := docling.NewDoclingExtractor(params.DoclingURL, client, logger)
	if err != nil {
		return fmt.Errorf("error creating docling extractor: %w", err)
	}

	err = extractstep.RunExtract(ctx, &sourceDocSource, &extracedDocSink, &extractor, logger)
	if err != nil {
		return err
	}

	return nil
}

type chunkParams struct {
	Flush bool
}

func RunChunking(ctx context.Context, db *sql.DB, params chunkParams, logger *slog.Logger) error {
	sink, err := chunk.NewResultSink(db, logger)
	if err != nil {
		return err
	}

	if params.Flush {
		if err := sqlite.FlushChunkTable(ctx, db); err != nil {
			return fmt.Errorf("error flushing chunks: %w", err)
		}
		if err := sqlite.FlushAllEmbeddings(ctx, db, logger); err != nil {
			return fmt.Errorf("error flushing embeddings_tables: %w", err)
		}
	}

	source, err := chunk.NewExtractedDocSource(db, logger)
	if err != nil {
		return err
	}

	err = chunkstep.Chunk(ctx, &source, sink, logger)
	if err != nil {
		return err
	}

	return nil
}

type embedParams struct {
	Dimensions int64
	Model      string
}

func runEmbedding(ctx context.Context, db *sql.DB, client cmd.EmbedClient, params embedParams, logger *slog.Logger) error {
	sink, err := embedding.NewEmbeddingsResultSink(db, logger)
	if err != nil {
		return err
	}
	tableName, err := sqlite.SetupVecTable(ctx, db, params.Dimensions, params.Model)
	if err != nil {
		return fmt.Errorf("error setting up sqlite vec tables: %w", err)
	}

	chunkSource, err := embedding.NewChunkSource(db, tableName, logger)
	if err != nil {
		return err
	}
	source := &chunkSource

	err = embedstep.Embed(ctx, sink, source, client, logger)
	if err != nil {
		return err
	}

	return nil
}
