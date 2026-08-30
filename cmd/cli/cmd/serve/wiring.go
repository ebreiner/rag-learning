package serve

import (
	"context"
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

type embedBackendConfig interface{ isEmbedBackendConfig() }

type xbergConfig struct {
	URL string
}

func (xbergConfig) isEmbedBackendConfig() {}

type openAIConfig struct {
	URL   string
	Model string
	Dim   int64
}

func (openAIConfig) isEmbedBackendConfig() {}

func wireUp(ctx context.Context, dbPath string, embedConfig embedBackendConfig, logger *slog.Logger) (deps step.RetrievalDeps, closeDB func(context.Context) error, err error) {
	db, err := sqlite.NewConn(dbPath, false)
	deps = step.RetrievalDeps{}
	if err != nil {
		return deps, nil, err
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
		return deps, closeDB, err
	}

	deps.Logger = logger

	hydrator, err := retrieval.NewChunkHydrator(ctx, db, logger)
	if err != nil {
		return deps, closeDB, err
	}
	deps.Hydrator = &hydrator

	retriever, err := retrieval.NewSQLiteRetriever(db, logger)
	if err != nil {
		return deps, closeDB, err
	}
	deps.Retriever = retriever

	embedClient, err := newEmbedClient(embedConfig, logger)
	if err != nil {
		return deps, closeDB, err
	}
	deps.EmbeddingsClient = embedClient

	renderer, err := retrieval.NewChunkRenderer(ctx, db, logger)
	if err != nil {
		return deps, closeDB, err
	}
	deps.Renderer = &renderer

	collWeigher, err := retrieval.NewCollectionWeigher(ctx, db, logger)
	if err != nil {
		return deps, closeDB, err
	}
	deps.CollWeigher = &collWeigher

	return deps, closeDB, nil
}

func newEmbedClient(cnf embedBackendConfig, logger *slog.Logger) (step.EmbedClient, error) {
	switch c := cnf.(type) {
	case xbergConfig:
		httpClient := httpclient.New(60 * time.Second)
		return kreuzberg.NewKreuzbergClient(c.URL, logger, httpClient)
	case openAIConfig:
		httpClient := httpclient.New(60 * time.Second)
		return openai.NewOpenAIClient(c.Model, c.URL, c.Dim, httpClient, logger)
	default:
		return openai.ClientOpenAI{}, fmt.Errorf("unknown embed backend")
	}
}
