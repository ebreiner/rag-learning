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

func NewChunkHydrator(db *sql.DB, logger *slog.Logger) ChunkHydrator {
	hydrator := ChunkHydrator{}
	hydrator.db = db
	hydrator.q = querries.New(hydrator.db)
	hydrator.Logger = logger

	return hydrator
}

func (h *ChunkHydrator) HydrateChunks(ctx context.Context, chunkIDs []step.ScoredChunkID) ([]step.RetrievedChunk, error) {
	chunks := make([]step.RetrievedChunk, 0, len(chunkIDs))

	ids := make([]int64, 0)
	for i := range chunkIDs {
		ids = append(ids, chunkIDs[i].ID)
	}

	rows, err := h.q.RetrievalChunksByIDs(ctx, ids)
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

	for idx, id := range ids {
		row, ok := rowByID[id]
		if !ok {
			h.Logger.WarnContext(ctx, "run-retrieval", "warn", fmt.Sprintf("chunk hydration failed: no row returned for chunk id %d, skipping chunk\n", id))
			continue
		}
		chunk := step.RetrievedChunk{
			Rank:       chunkIDs[idx].Rank,
			DocTitle:   row.Name,
			Position:   row.Position,
			Text:       row.Text,
			ID:         row.ID,
			Breadcrumb: row.Breadcrumb,
			Score:      chunkIDs[idx].Score,
		}

		chunk.CollectionWeight = row.Weight
		chunk.CollectionName = row.CollectionName

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}
