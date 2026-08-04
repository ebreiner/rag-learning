package retrieval

import (
	"database/sql"
	"errors"
	"fmt"
	"rag/internal/platform/sqlite"
	"rag/internal/retrieval/step"
)

func (r *SQLiteRetriever) TopKByANN(embedding []float64, k int64) (step.RetrievedChunkIDs, error) {
	chunkIDs := make([]int64, 0)

	packed, err := sqlite.PackVector(embedding)
	if err != nil {
		return chunkIDs, err
	}

	query := `
	SELECT e.chunk_id, e.distance
	FROM embeddings_balanced_1536 AS e
	WHERE e.embedding MATCH ?
	ORDER BY e.distance
	LIMIT ?
	`
	rows, err := r.db.QueryContext(r.ctx, query, packed, k)
	if errors.Is(err, sql.ErrNoRows) {
		return chunkIDs, fmt.Errorf("error: no rows found")
	}
	if err != nil {
		return chunkIDs, fmt.Errorf("error querring rows: %s", err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var distance sql.NullFloat64
		if err := rows.Scan(&id, &distance); err != nil {
			return chunkIDs, fmt.Errorf("error scanning top k row result: %s", err.Error())
		}
		chunkIDs = append(chunkIDs, id)
	}

	return chunkIDs, nil
}
