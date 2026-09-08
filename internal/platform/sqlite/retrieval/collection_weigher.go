package retrieval

import (
	"context"
	"database/sql"
	"log/slog"
	"rag/internal/platform/sqlite/querries"
	"rag/internal/retrieval/step"
)

type CollectionWeigher struct {
	db     *sql.DB
	q      *querries.Queries
	Logger *slog.Logger
}

func NewCollectionWeigher(db *sql.DB, logger *slog.Logger) CollectionWeigher {
	weigher := CollectionWeigher{}
	weigher.db = db
	weigher.q = querries.New(db)
	weigher.Logger = logger

	return weigher
}

func (c *CollectionWeigher) WeighChunks(ctx context.Context, chunkIDs []step.ScoredChunkID) (map[int64]float64, error) {
	ids := make([]int64, 0, len(chunkIDs))
	for _, c := range chunkIDs {
		ids = append(ids, c.ID)
	}

	rows, err := c.q.CollectionWeightForChunkIDs(ctx, ids)
	if err != nil {
		return nil, err
	}

	weightByID := make(map[int64]float64, len(rows))
	for _, row := range rows {
		weightByID[row.ID] = row.Weight
	}
	return weightByID, nil
}
