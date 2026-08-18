package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"rag/internal/platform/sqlite/querries"
	"rag/internal/retrieval/step"
)

type ChunkHydrator struct {
	db     *sql.DB
	q      *querries.Queries
	Logger *slog.Logger
}

func NewChunkHydrator(db *sql.DB, logger *slog.Logger) (ChunkHydrator, error) {
	hydrator := ChunkHydrator{}
	hydrator.db = db
	hydrator.q = querries.New(hydrator.db)
	hydrator.Logger = logger

	return hydrator, nil
}

// TODO: rank explizit über boundaries transportieren und nicht nur auf implizites ordering verlassen
func (h *ChunkHydrator) HydrateChunks(chunkIDs step.RetrievedChunkIDs, ctx context.Context) ([]step.RetrievedChunk, error) {
	chunks := make([]step.RetrievedChunk, 0, len(chunkIDs))

	rows, err := h.q.RetrievalChunksByIDs(ctx, chunkIDs)
	if err != nil {
		return chunks, err
	}
	if len(rows) == 0 {
		return chunks, nil
	}

	rowByID := make(map[int64]querries.RetrievalChunksByIDsRow, 0)
	for _, row := range rows {
		rowByID[row.ID] = row
	}

	for idx, id := range chunkIDs {
		row, ok := rowByID[id]
		if !ok {
			h.Logger.WarnContext(ctx, "run-retrieval", "warn", fmt.Sprintf("chunk hydration failed: no row returned for chunk id %d, skipping chunk\n", id))
			continue
		}
		chunk := step.RetrievedChunk{
			Rank:     int64(idx + 1),
			DocTitle: row.Name,
			Position: row.Position,
			Text:     row.Text,
			ID:       row.ID,
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
