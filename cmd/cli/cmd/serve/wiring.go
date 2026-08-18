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

type xbergConfig struct{ URL string }

func (xbergConfig) isEmbedBackendConfig() {}

type openAIConfig struct {
	URL   string
	Model string
	Dim   int64
}

func (openAIConfig) isEmbedBackendConfig() {}

func wireUp(dbPath string, embedConfig embedBackendConfig, ctx context.Context, logger *slog.Logger) (
	hydrator retrieval.ChunkHydrator, retriever step.TopKRetriever, embedClient step.EmbedClient, closeDB func(context.Context) error, err error) {
	db, err := sqlite.NewConn(dbPath)
	if err != nil {
		return retrieval.ChunkHydrator{}, &retrieval.SQLiteRetriever{}, openai.ClientOpenAI{}, nil, err
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
		return retrieval.ChunkHydrator{}, &retrieval.SQLiteRetriever{}, openai.ClientOpenAI{}, closeDB, err
	}
	hydrator, err = retrieval.NewChunkHydrator(db, logger)
	if err != nil {
		return retrieval.ChunkHydrator{}, &retrieval.SQLiteRetriever{}, openai.ClientOpenAI{}, closeDB, err
	}

	retriever, err = retrieval.NewSQLiteRetriever(db, logger)
	if err != nil {
		return retrieval.ChunkHydrator{}, &retrieval.SQLiteRetriever{}, openai.ClientOpenAI{}, closeDB, err
	}

	embedClient, err = newEmbedClient(embedConfig, logger)
	if err != nil {
		return retrieval.ChunkHydrator{}, &retrieval.SQLiteRetriever{}, openai.ClientOpenAI{}, closeDB, err
	}

	return hydrator, retriever, embedClient, closeDB, nil
}

func newEmbedClient(cnf embedBackendConfig, logger *slog.Logger) (step.EmbedClient, error) {
	switch c := cnf.(type) {
	case xbergConfig:
		return kreuzberg.NewKreuzbergClient(c.URL, logger)
	case openAIConfig:
		httpClient := httpclient.New(60 * time.Second)
		return openai.NewOpenAIClient(c.Model, c.URL, c.Dim, httpClient, logger)
	default:
		return openai.ClientOpenAI{}, fmt.Errorf("unknown embed backend")
	}
}
