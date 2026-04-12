package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type ChunkList map[uint16]Chunk

type Chunk struct {
	ID       int64
	Rank     uint16
	Distance float64
	Text     string
	DocID    int64
	DocTitle string
}

func TopKByVec(db *sql.DB, k int, embedding []float32, ctx context.Context) (ChunkList, error) {
	chunks := make(ChunkList)
	buf, err := PackEmbedding(embedding)
	if err != nil {
		return chunks, fmt.Errorf("error writing float to buffer: %s", err.Error())
	}

	query := `
	SELECT e.chunk_id, e.distance
	FROM embeddings_balanced_1536 AS e
	WHERE e.embedding MATCH ?
	ORDER BY e.distance
	LIMIT ?
	`
	rows, err := db.QueryContext(ctx, query, buf.Bytes(), k)
	defer rows.Close()
	if errors.Is(err, sql.ErrNoRows) {
		return chunks, fmt.Errorf("error: no rows found")
	}
	if err != nil {
		return chunks, fmt.Errorf("error querring rows: %s", err.Error())
	}
	var rank uint16
	rank = 1
	for rows.Next() {
		var chunk Chunk
		if err := rows.Scan(&chunk.ID, &chunk.Distance); err != nil {
			return chunks, fmt.Errorf("error scanning top k row result: %s", err.Error())
		}
		chunk.Rank = rank
		chunks[uint16(rank)] = chunk
		rank = rank + 1
	}
	return chunks, nil
}

func TopKByFts(db *sql.DB, k uint16, rawQuery string, ctx context.Context) (ChunkList, error) {
	parts := strings.Fields(rawQuery)
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		if len([]rune(part)) > 2 {
			terms = append(terms, part)
		} else {
			// spaces werden automatisch als AND interpretiert von fts. daher filtern wir relevante (wörter länger 2) raus und
			// bauen eine liste. diese werden dann via OR verkettet.
			continue
		}
	}
	// Schadet nicht und kann Präzison erhöhen
	terms = append(terms, rawQuery)
	matchTerm := ""
	for idx, el := range terms {
		if idx == len(terms)-1 {
			matchTerm = matchTerm + fmt.Sprintf("%s'", el)
			continue
		}
		if idx == 0 {
			matchTerm = fmt.Sprintf("'%s OR ", el)
			continue
		}

		matchTerm = matchTerm + fmt.Sprintf("%s OR ", el)
	}

	chunks := make(ChunkList)
	q := `
	SELECT rowid, bm25(chunks_fts)
	FROM chunks_fts
	WHERE chunks_fts MATCH %s
	ORDER BY bm25(chunks_fts)
	LIMIT ?
	`
	q = fmt.Sprintf(q, matchTerm)
	rows, err := db.QueryContext(ctx, q, k)
	if err != nil {
		return chunks, fmt.Errorf("error querring rows: %s", err.Error())
	}
	defer rows.Close()

	var rank uint16
	rank = 1
	for rows.Next() {
		if err := rows.Err(); err != nil {
			return chunks, fmt.Errorf("error scanning rows for fts: %s", err.Error())
		}
		var chunk Chunk
		if err := rows.Scan(&chunk.ID, &chunk.Distance); err != nil {
			return chunks, fmt.Errorf("error scanning top k row result: %s", err.Error())
		}
		chunk.Rank = rank
		chunks[rank] = chunk
		rank = rank + 1
	}
	return chunks, nil
}

func ChunksForIDs(ctx context.Context, db *sql.DB, chunks ChunkList) (ChunkList, error) {
	return chunks, nil
}
