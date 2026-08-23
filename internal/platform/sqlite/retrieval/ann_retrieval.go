package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"rag/internal/platform/sqlite"
	"rag/internal/retrieval/step"
)

func (r *SQLiteRetriever) TopKByANN(ctx context.Context, query step.Query, k int64) (step.RetrievedChunkIDs, error) {
	chunkIDs := make([]int64, 0)

	packed, err := sqlite.PackVector(query.Vector)
	if err != nil {
		return chunkIDs, err
	}

	tableName, err := sqlite.LookupVecTable(ctx, r.db, query.Dim, query.Model)
	if err != nil {
		return chunkIDs, err
	}

	q := fmt.Sprintf("SELECT e.chunk_id, e.distance FROM %s AS e WHERE e.embedding MATCH ? ORDER BY e.distance LIMIT ?", tableName)

	rows, err := r.db.QueryContext(ctx, q, packed, k)
	if errors.Is(err, sql.ErrNoRows) {
		return chunkIDs, fmt.Errorf("error: no rows found")
	}
	if err != nil {
		return chunkIDs, fmt.Errorf("error querring rows: %w", err)
	}

	defer func() {
		if err := rows.Close(); err != nil {
			r.Logger.ErrorContext(ctx, "close-db", "err", err)
		}

	}()

	for rows.Next() {
		var id int64
		var distance sql.NullFloat64
		if err := rows.Scan(&id, &distance); err != nil {
			return chunkIDs, fmt.Errorf("error scanning top k row result: %w", err)
		}
		chunkIDs = append(chunkIDs, id)
	}
	if rows.Err() != nil {
		return chunkIDs, rows.Err()
	} else {
		return chunkIDs, nil
	}
}
