package retrieval

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"rag/internal/retrieval/step"
)

func ReadCatalog(ctx context.Context, db *sql.DB) (step.Catalog, error) {
	sources, err := sourceCounts(ctx, db)
	if err != nil {
		return step.Catalog{}, fmt.Errorf("reading source counts: %w", err)
	}
	rows, err := catalogRows(ctx, db)
	if err != nil {
		return step.Catalog{}, fmt.Errorf("reading metadata: %w", err)
	}
	return step.BuildCatalog(sources, rows), nil
}

func sourceCounts(ctx context.Context, db *sql.DB) (counts []step.SourceCount, err error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.name, COUNT(d.id)
		FROM sources AS s
		LEFT JOIN documents AS d ON d.source_name = s.name
		GROUP BY s.name
		ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var c step.SourceCount
		if err := rows.Scan(&c.Name, &c.Documents); err != nil {
			return nil, err
		}
		counts = append(counts, c)
	}
	return counts, rows.Err()
}

func catalogRows(ctx context.Context, db *sql.DB) (result []step.CatalogRow, err error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.source_name, j.key, j.type, j.value, COUNT(*)
		FROM documents AS d, json_each(d.metadata_json) AS j
		WHERE d.metadata_json IS NOT NULL
		GROUP BY d.source_name, j.key, j.type, j.value`)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var row step.CatalogRow
		var jsonType string
		var raw any
		if err := rows.Scan(&row.Source, &row.Key, &jsonType, &raw, &row.Documents); err != nil {
			return nil, err
		}
		row.Type, row.Value = catalogValue(jsonType, raw)
		result = append(result, row)
	}
	return result, rows.Err()
}

func catalogValue(jsonType string, raw any) (string, any) {
	switch jsonType {
	case "true":
		return "boolean", true
	case "false":
		return "boolean", false
	case "integer", "real":
		return jsonType, raw
	case "text":
		if b, ok := raw.([]byte); ok {
			return jsonType, string(b)
		}
		return jsonType, raw
	default: // object, array, null
		return jsonType, nil
	}
}
