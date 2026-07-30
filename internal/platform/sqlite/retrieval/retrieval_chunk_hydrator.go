package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"rag/internal/platform/sqlite/querries"
	"rag/internal/retrieval/step"
)

type ChunkHydrator struct {
	db  *sql.DB
	q   *querries.Queries
	ctx context.Context
}

func NewChunkHydrator(db *sql.DB, ctx context.Context) (ChunkHydrator, error) {
	hydrator := ChunkHydrator{}
	hydrator.db = db
	hydrator.ctx = ctx
	hydrator.q = querries.New(hydrator.db)

	return hydrator, nil
}

func (h *ChunkHydrator) HydrateChunks(chunkIDs step.RetrievedChunkIDs) ([]step.RetrievedChunk, error) {
	chunks := make([]step.RetrievedChunk, 0)

	rows, err := h.q.RetrievalChunksByIDs(h.ctx, chunkIDs)
	if err != nil {
		return chunks, err
	}
	if len(rows) == 0 {
		return chunks, fmt.Errorf("query for chunk hydration did not return rows")
	}

	for idx, row := range rows {
		var rank int64
		rank = int64(idx) + 1
		chunk := step.RetrievedChunk{
			Rank:     rank,
			DocTitle: row.Name,
			Position: row.Position,
			Text:     row.Text,
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
