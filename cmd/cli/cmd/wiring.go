package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"rag/internal/platform/embedclient/kreuzberg"
	"rag/internal/platform/embedclient/openai"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/retrieval"
	"rag/internal/retrieval/step"
	"time"
)

type EmbedBackendConfig interface{ isEmbedBackendConfig() }

type XbergConfig struct {
	URL string
}

func (XbergConfig) isEmbedBackendConfig() {}

type OpenAIConfig struct {
	URL   string
	Model string
	Dim   int64
}

func (OpenAIConfig) isEmbedBackendConfig() {}

func WireUpRetrieval(ctx context.Context, dbPath string, embedConfig EmbedBackendConfig, logger *slog.Logger) (deps step.RetrievalDeps, db *sql.DB, closeDB func(context.Context) error, err error) {
	db, err = sqlite.NewConn(dbPath, false)
	deps = step.RetrievalDeps{}
	if err != nil {
		return deps, db, nil, err
	}

	closeDB = func(ctx context.Context) error {
		done := make(chan error, 1)
		go func() { done <- db.Close() }()
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if err != nil {
		return deps, db, closeDB, err
	}

	deps.Logger = logger

	hydrator := retrieval.NewChunkHydrator(db, logger)
	deps.Hydrator = &hydrator

	retriever, err := retrieval.NewSQLiteRetriever(db, logger)
	if err != nil {
		return deps, db, closeDB, err
	}
	deps.Retriever = retriever

	embedClient, err := newEmbedClient(embedConfig, logger)
	if err != nil {
		return deps, db, closeDB, err
	}
	deps.EmbeddingsClient = embedClient

	renderer, err := retrieval.NewChunkRenderer(ctx, db, logger)
	if err != nil {
		return deps, db, closeDB, err
	}
	deps.Renderer = &renderer

	collWeigher := retrieval.NewCollectionWeigher(db, logger)
	deps.CollWeigher = &collWeigher

	return deps, db, closeDB, nil
}

func newEmbedClient(cnf EmbedBackendConfig, logger *slog.Logger) (step.EmbedClient, error) {
	switch c := cnf.(type) {
	case XbergConfig:
		httpClient := httpclient.New(60 * time.Second)
		return kreuzberg.NewKreuzbergClient(c.URL, logger, httpClient)
	case OpenAIConfig:
		httpClient := httpclient.New(60 * time.Second)
		return openai.NewOpenAIClient(c.Model, c.URL, c.Dim, httpClient, logger)
	default:
		return openai.ClientOpenAI{}, fmt.Errorf("unknown embed backend")
	}
}
