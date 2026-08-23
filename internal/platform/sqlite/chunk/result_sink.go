package chunk

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/querries"
	"time"
)

type ResultSink struct {
	dbClient *sql.DB
	Logger   *slog.Logger
}

func NewResultSink(db *sql.DB, logger *slog.Logger) (ResultSink, error) {
	store := ResultSink{}
	store.dbClient = db
	store.Logger = logger

	return store, nil
}

func (store ResultSink) SaveChunks(ctx context.Context, chunkResult step.ChunkResult) error {

	tx, err := store.dbClient.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q := querries.New(tx)

	for index := range chunkResult.ChunksToSave {
		param := querries.InsertChunkParams{
			Text:       chunkResult.ChunksToSave[index].Text,
			DocumentID: chunkResult.DocumentID,
			CreatedAt:  time.Now(),
			Position:   chunkResult.ChunksToSave[index].Position,
			Breadcrumb: chunkResult.ChunksToSave[index].Breadcrumb,
		}
		err := q.InsertChunk(ctx, param)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}
