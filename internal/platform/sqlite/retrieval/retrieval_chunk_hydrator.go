package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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

// TODO: rank explizit über boundaries transportieren und nicht nur auf implizites ordering verlassen
func (h *ChunkHydrator) HydrateChunks(chunkIDs step.RetrievedChunkIDs) ([]step.RetrievedChunk, error) {
	chunks := make([]step.RetrievedChunk, 0, len(chunkIDs))

	rows, err := h.q.RetrievalChunksByIDs(h.ctx, chunkIDs)
	if err != nil {
		return chunks, err
	}
	if len(rows) == 0 {
		return chunks, fmt.Errorf("query for chunk hydration did not return rows")
	}

	rowByID := make(map[int64]querries.RetrievalChunksByIDsRow, 0)
	for _, row := range rows {
		rowByID[row.ID] = row
	}

	for idx, id := range chunkIDs {
		row, ok := rowByID[id]
		if !ok {
			log.Printf("chunk hydration failed: no row returned for chunk id %d, skipping chunk\n", id)
			continue
		}
		chunk := step.RetrievedChunk{
			Rank:     int64(idx + 1),
			DocTitle: row.Name,
			Position: row.Position,
			Text:     row.Text,
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
