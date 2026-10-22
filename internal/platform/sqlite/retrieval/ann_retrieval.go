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
		// filter inside the KNN with chunk_id IN, not with a join, see knowledge/concepts/sqlite-query-plans.md
		clause, filterArgs := filterClause(filter)
		q = fmt.Sprintf(`WITH knn AS MATERIALIZED (
				SELECT e.chunk_id, e.distance
				FROM %s AS e
				WHERE e.embedding MATCH ? AND e.k = ?
				AND e.chunk_id IN (
					SELECT c.id
					FROM chunks AS c
					JOIN documents AS d ON d.id = c.document_id
					WHERE 1 = 1%s)
			)
			SELECT chunk_id, distance FROM knn ORDER BY distance, chunk_id`, tableName, clause)
		args = append([]any{packed, k}, filterArgs...)
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
