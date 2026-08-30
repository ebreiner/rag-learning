package retrieval

import (
	"context"
	"database/sql"
	"log/slog"
	"rag/internal/platform/sqlite/querries"
	"rag/internal/retrieval/step"
)

type ChunkRenderer struct {
	db     *sql.DB
	q      *querries.Queries
	Logger *slog.Logger
}

func NewChunkRenderer(ctx context.Context, db *sql.DB, logger *slog.Logger) (ChunkRenderer, error) {
	renderer := ChunkRenderer{}
	renderer.db = db
	renderer.q = querries.New(renderer.db)
	renderer.Logger = logger

	return renderer, nil
}

func (r *ChunkRenderer) RenderChunks(ctx context.Context, chunks []step.RetrievedChunk) ([]step.RetrievedChunk, error) {
	// TODO: rewrite with real renderer, right now utilizing hydrated chunks present chunker text
	// noop
	return chunks, nil
}
