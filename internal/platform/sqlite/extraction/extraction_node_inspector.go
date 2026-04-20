package extraction

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"rag/internal/platform/sqlite/querries"
)

type ExtractionNodeInspector struct {
	Limit  int
	Format string
	db     *sql.DB
	ctx    context.Context
}

func NewExtractionNodeInspector(ctx context.Context, db *sql.DB, limit int, format string) ExtractionNodeInspector {
	return ExtractionNodeInspector{
		Limit:  limit,
		Format: format,
		db:     db,
		ctx:    ctx,
	}
}

func (i ExtractionNodeInspector) Stats() (string, error) {
	q := querries.New(i.db)
	count, err := q.CountExtractionNodes(i.ctx)
	if err != nil {
		return "", err
	}
	stats := stats{Type: "extraction_nodes:", Count: count}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func (i ExtractionNodeInspector) Dump() (string, error) {
	q := querries.New(i.db)
	rows, err := q.HeadExtractionNodes(i.ctx, int64(i.Limit))
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
		for _, node := range rows {
			s, err := json.Marshal(node)
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
