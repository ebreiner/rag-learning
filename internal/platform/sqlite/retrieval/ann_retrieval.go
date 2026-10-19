package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"rag/internal/platform/sqlite"
	"rag/internal/retrieval/step"
)

func (r *SQLiteRetriever) TopKByANN(ctx context.Context, query step.Query, k int64, filter step.Filter) ([]step.ScoredChunkID, error) {
	chunkIDs := make([]step.ScoredChunkID, 0)

	packed, err := sqlite.PackVector(query.Vector)
	if err != nil {
		return chunkIDs, err
	}

	tableName, err := sqlite.LookupVecTable(ctx, r.db, query.Dim, query.Model)
	if err != nil {
		return chunkIDs, err
	}

	q := fmt.Sprintf("SELECT e.chunk_id, e.distance FROM %s AS e WHERE e.embedding MATCH ? ORDER BY e.distance LIMIT ?", tableName)
	args := []any{packed, k}
	if !filter.IsEmpty() {
		// the KNN query applies a WHERE on joined tables only after the k nearest chunks,
		// so a filter needs a full scan of the filtered chunks, L2 is the vec0 default  order stays the same
		clause, filterArgs := filterClause(filter)
		q = fmt.Sprintf(`SELECT e.chunk_id, vec_distance_l2(e.embedding, ?) AS distance
			FROM %s AS e
			JOIN chunks AS c ON c.id = e.chunk_id
			JOIN documents AS d ON d.id = c.document_id
			WHERE 1 = 1%s
			ORDER BY distance
			LIMIT ?`, tableName, clause)
		args = append(append([]any{packed}, filterArgs...), k)
	}

	rows, err := r.db.QueryContext(ctx, q, args...)
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
		if distance.Valid {
			chunkIDs = append(chunkIDs, step.ScoredChunkID{ID: id, Score: distance.Float64})
		} else {
			return chunkIDs, fmt.Errorf("empty distance for ANN received")
		}
	}
	if rows.Err() != nil {
		return chunkIDs, rows.Err()
	} else {
		return chunkIDs, nil
	}
}
