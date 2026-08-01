package embedding

import (
	"context"
	"database/sql"
	"fmt"
	"rag/internal/embedding/step"
	"rag/internal/platform/sqlite/querries"
)

type ChunksSource struct {
	dbClient *sql.DB
	ctx      context.Context
	lastID   int64
}

func NewChunkSource(db *sql.DB, ctx context.Context) (ChunksSource, error) {
	source := ChunksSource{}
	source.dbClient = db
	source.ctx = ctx
	source.lastID = 0

	return source, nil
}

func (s *ChunksSource) NextChunks(limit int64) ([]step.ChunkToEmbed, error) {
	q := querries.New(s.dbClient)
	param := querries.GetChunkBatchAfterIDParams{
		ID:    s.lastID,
		Limit: limit,
	}

	chunkRows, err := q.GetChunkBatchAfterID(s.ctx, param)
	if err != nil {
		return nil, err
	}
	if len(chunkRows) == 0 {
		fmt.Println("no rows returned")
		return nil, nil
	}

	nextChunks := make([]step.ChunkToEmbed, 0, len(chunkRows))
	for _, row := range chunkRows {
		nextChunks = append(nextChunks, step.ChunkToEmbed{
			ChunkID: row.ID,
			Text:    row.Text,
		})
	}

	s.lastID = chunkRows[len(chunkRows)-1].ID

	return nextChunks, nil
}
