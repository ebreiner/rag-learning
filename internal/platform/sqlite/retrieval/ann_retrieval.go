package retrieval

import (
	"bytes"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"rag/internal/retrieval/step"
)

func (r *SQLiteRetriever) TopKByANN(embedding []float64, k int64) (step.RetrievedChunkIDs, error) {
	chunkIDs := make([]int64, 0)

	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, embedding); err != nil {
		return chunkIDs, fmt.Errorf("error converting embedding to buffer: %s", err.Error())
	}

	query := `
	SELECT e.chunk_id, e.distance
	FROM embeddings_balanced_1536 AS e
	WHERE e.embedding MATCH ?
	ORDER BY e.distance
	LIMIT ?
	`
	rows, err := r.db.QueryContext(r.ctx, query, buf.Bytes(), k)
	defer rows.Close()
	if errors.Is(err, sql.ErrNoRows) {
		return chunkIDs, fmt.Errorf("error: no rows found")
	}
	if err != nil {
		return chunkIDs, fmt.Errorf("error querring rows: %s", err.Error())
	}
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
