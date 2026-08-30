package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"rag/internal/platform/sqlite/querries"
	"rag/internal/retrieval/step"
)

type CollectionWeigher struct {
	db          *sql.DB
	q           *querries.Queries
	Logger      *slog.Logger
	collections map[string]float64
}

func NewCollectionWeigher(ctx context.Context, db *sql.DB, logger *slog.Logger) (CollectionWeigher, error) {
	rows, err := querries.New(db).GetAllCollectionWeights(ctx)
	if err != nil {
		return CollectionWeigher{}, err
	}
	if len(rows) == 0 {
		return CollectionWeigher{}, fmt.Errorf("empty collection table")
	}

	collMap := make(map[string]float64)
	for _, row := range rows {
		collMap[row.Name] = row.Weight
	}

	weigher := CollectionWeigher{collections: collMap}
	weigher.db = db
	weigher.q = querries.New(db)
	weigher.Logger = logger

	return weigher, nil
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
