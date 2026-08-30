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
		chunk := chunkResult.ChunksToSave[index]
		chunkParam := querries.InsertChunkParams{
			Text:       chunk.Text,
			DocumentID: chunkResult.DocumentID,
			CreatedAt:  time.Now(),
			Position:   chunk.Position,
			Breadcrumb: chunk.Breadcrumb,
			Type:       string(chunk.Type),
		}
		chunkID, err := q.InsertChunk(ctx, chunkParam)
		if err != nil {
			return err
		}

		for _, extID := range chunkResult.ChunksToSave[int(index)].ExtractionNodeIDs {
			param := querries.InsertChunkNodesParams{
				ChunkID:          chunkID,
				ExtractionNodeID: extID.ExtractionNodeID,
				CreatedAt:        time.Now(),
				Position:         extID.Position,
			}
			if err := q.InsertChunkNodes(ctx, param); err != nil {
				return fmt.Errorf("error inserting chunk_nodes: %w", err)
			}
		}

	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	return nil
}
