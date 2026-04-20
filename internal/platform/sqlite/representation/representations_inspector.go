package representation

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"rag/internal/platform/sqlite/querries"
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
	q := querries.New(i.db)
	count, err := q.CountRepresentations(i.ctx)
	if err != nil {
		return "", err
	}
	stats := stats{Type: "representations", Count: count}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func (i Inspector) Dump() (string, error) {
	q := querries.New(i.db)
	rows, err := q.HeadRepresentations(i.ctx, int64(i.Limit))
	if err != nil {
		return "", err
	}
	var output string
	switch i.Format {
	case "json":
		s, err := json.Marshal(rows)
		if err != nil {
			return "", err
		}
		output = string(s)
	case "jsonl":
		for _, representation := range rows {
			s, err := json.Marshal(representation)
			if err != nil {
				return "", err
			}
			output = output + string(s)
		}
	default:
		return "", fmt.Errorf("unknown output format '%s'", i.Format)
	}
	return output, nil
}
