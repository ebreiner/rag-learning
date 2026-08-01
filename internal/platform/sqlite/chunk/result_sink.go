package chunk

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/querries"
	"time"
)

type ResultSink struct {
	dbClient *sql.DB
	ctx      context.Context
}

func NewResultSink(db *sql.DB, ctx context.Context) (ResultSink, error) {
	store := ResultSink{}
	store.dbClient = db
	store.ctx = ctx

	return store, nil
}

func (store ResultSink) SaveChunks(chunkResult step.ChunkResult) error {
	parent := sql.NullInt64{Int64: chunkResult.ParentRepresentationID, Valid: true}
	representationParams := querries.CreateChildRepresentationFromParentParams{
		Stage:                  "chunk",
		CreatedAt:              time.Now(),
		ParentRepresentationID: parent,
		ID:                     parent.Int64, // Also parent, check query
	}
	tx, err := store.dbClient.BeginTx(store.ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	q := querries.New(tx)

	fmt.Printf("parent rep id = %d\n", chunkResult.ParentRepresentationID)
	representationID, err := q.CreateChildRepresentationFromParent(store.ctx, representationParams)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf(
				"parent representation %d not found when creating chunk representation",
				chunkResult.ParentRepresentationID,
			)
		}
		return fmt.Errorf("error creating child representation of type chunk: %w", err)
	}
	for index := range chunkResult.ChunksToSave {
		param := querries.InsertChunkParams{
			RepresentationID: representationID,
			Text:             chunkResult.ChunksToSave[index].Text,
			Position:         chunkResult.ChunksToSave[index].Position,
			CreatedAt:        time.Now(),
		}
		err := q.InsertChunk(store.ctx, param)
		if err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("rolling back transaction: %s", err.Error())
	}

	return nil
}
