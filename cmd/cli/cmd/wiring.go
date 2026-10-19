package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	embedstep "rag/internal/embedding/step"
	"rag/internal/platform/config"
	"rag/internal/platform/embedclient/kreuzberg"
	"rag/internal/platform/embedclient/openai"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/retrieval"
	retrievalstep "rag/internal/retrieval/step"
)

type EmbedClient interface {
	embedstep.EmbedClient
	retrievalstep.EmbedClient
}

func NewEmbedClient(cnf config.Embedding, logger *slog.Logger) (EmbedClient, error) {
	httpClient := httpclient.New(cnf.Timeout, nil)
	switch cnf.Backend {
	case config.EmbedBackendOpenAI:
		return openai.NewOpenAIClient(cnf.Model, cnf.URL.String(), cnf.Dim, httpClient, logger)
	case config.EmbedBackendKreuzberg:
		return kreuzberg.NewKreuzbergClient(cnf.URL.String(), logger, httpClient)
	default:
		return nil, fmt.Errorf("unknown embed backend %q", cnf.Backend)
	}
}

func WireUpRetrieval(ctx context.Context, cnf config.Config, logger *slog.Logger) (deps retrievalstep.RetrievalDeps, db *sql.DB, closeDB func(context.Context) error, err error) {
	db, err = sqlite.NewConn(cnf.Storage.DBPath, false)
	deps = retrievalstep.RetrievalDeps{}
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

	deps.Logger = logger

	hydrator := retrieval.NewChunkHydrator(db, logger)
	deps.Hydrator = &hydrator

	retriever, err := retrieval.NewSQLiteRetriever(db, logger)
	if err != nil {
		return deps, db, closeDB, err
	}
	deps.Retriever = retriever

	embedClient, err := NewEmbedClient(cnf.Embedding, logger)
	if err != nil {
		return deps, db, closeDB, err
	}
	deps.EmbeddingsClient = embedClient

	renderer, err := retrieval.NewChunkRenderer(ctx, db, logger)
	if err != nil {
		return deps, db, closeDB, err
	}
	deps.Renderer = &renderer

	deps.Fusion = retrievalstep.FusionConfig(cnf.Retrieval.Fusion)
	deps.PoolSize = cnf.Retrieval.PoolSize
	for _, s := range cnf.Sources {
		deps.Sources = append(deps.Sources, s.Name)
	}
	logger.InfoContext(ctx, "wiring", "info", fmt.Sprintf("hybrid: pool size %d, rrf k %d, weight fts %g, weight ann %g", deps.PoolSize, deps.Fusion.RRFK, deps.Fusion.WeightFTS, deps.Fusion.WeightANN))

	return deps, db, closeDB, nil
}

func NewReadDeps(db *sql.DB, cnf config.Config, logger *slog.Logger) retrievalstep.ReadDeps {
	loader := retrieval.NewDocumentLoader(db, logger)
	deps := retrievalstep.ReadDeps{Loader: &loader, MaxChars: cnf.Read.MaxChars}
	for _, s := range cnf.Sources {
		deps.Sources = append(deps.Sources, s.Name)
	}
	return deps
}
