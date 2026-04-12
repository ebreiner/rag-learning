package embedding

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type stats struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type Inspector struct {
	Limit  int
	Format string
	db     *sql.DB
	ctx    context.Context
}

func NewInspector(ctx context.Context, db *sql.DB, limit int, format string) Inspector {
	return Inspector{
		Limit:  limit,
		Format: format,
		db:     db,
		ctx:    ctx,
	}
}

func (i Inspector) Stats() (string, error) {
	var count int64
	q := "SELECT COUNT(chunk_id) FROM embeddings_balanced_1536"
	err := i.db.QueryRowContext(i.ctx, q).Scan(&count)
	if err != nil {
		return "", err
	}
	stats := stats{Type: "embeddings", Count: count}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func (i Inspector) Dump() (string, error) {
	q := "SELECT chunk_id FROM embeddings_balanced_1536 ORDER BY chunk_id LIMIT ?"
	rows, err := i.db.QueryContext(i.ctx, q, i.Limit)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	type embedding struct {
		ChunkID int64 `json:"chunk_id"`
	}

	embeddings := make([]embedding, 0, len(ids))
	for _, id := range ids {
		embeddings = append(embeddings, embedding{ChunkID: id})
	}

	switch i.Format {
	case "json":
		s, err := json.Marshal(embeddings)
		if err != nil {
			return "", err
		}
		return string(s), nil

	case "jsonl":
		var output string
		for _, e := range embeddings {
			s, err := json.Marshal(e)
			if err != nil {
				return "", err
			}
			output += string(s) + "\n"
		}
		return output, nil

	default:
		return "", fmt.Errorf("unknown output format '%s'", i.Format)
	}
}
