package retrieval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"rag/internal/retrieval/step"
	"slices"
	"strings"
)

type LinkLoader struct {
	db     *sql.DB
	Logger *slog.Logger
}

func NewLinkLoader(db *sql.DB, logger *slog.Logger) LinkLoader {
	return LinkLoader{db: db, Logger: logger}
}

const firstChunkOfD = `(SELECT c.id FROM chunks AS c WHERE c.document_id = d.id ORDER BY c.position, c.id LIMIT 1)`

func (l *LinkLoader) LoadLinks(ctx context.Context, docIDs []int64, filter step.Filter) (step.LinkData, error) {
	data := step.LinkData{
		Out:      map[int64][]step.LinkedDoc{},
		In:       map[int64][]step.LinkedDoc{},
		Sections: map[int64][]step.SectionStart{},
	}
	if len(docIDs) == 0 {
		return data, nil
	}

	in, idArgs := inClause(docIDs)
	clause, filterArgs := filterClause(filter)

	out := `
		SELECT l.from_document_id, l.section, COALESCE(l.text, ''), d.name, d.metadata_json, ` + firstChunkOfD + `
		FROM document_links AS l
		JOIN documents AS d ON d.source_name = l.to_source AND d.rel_path = l.to_rel_path
		WHERE l.from_document_id IN ` + in + clause + `
		ORDER BY l.from_document_id, l.position`
	if err := l.queryLinks(ctx, out, slices.Concat(idArgs, filterArgs), data.Out); err != nil {
		return data, fmt.Errorf("loading outgoing links: %w", err)
	}

	incoming := `
		SELECT t.id, l.section, COALESCE(l.text, ''), d.name, d.metadata_json, ` + firstChunkOfD + `
		FROM documents AS t
		JOIN document_links AS l ON l.to_source = t.source_name AND l.to_rel_path = t.rel_path
		JOIN documents AS d ON d.id = l.from_document_id
		WHERE t.id IN ` + in + clause + `
		ORDER BY t.id, l.from_document_id, l.position`
	if err := l.queryLinks(ctx, incoming, slices.Concat(idArgs, filterArgs), data.In); err != nil {
		return data, fmt.Errorf("loading incoming links: %w", err)
	}

	if err := l.querySections(ctx, in, idArgs, data.Sections); err != nil {
		return data, fmt.Errorf("loading section starts: %w", err)
	}

	return data, nil
}

func (l *LinkLoader) queryLinks(ctx context.Context, query string, args []any, into map[int64][]step.LinkedDoc) (err error) {
	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var docID int64
		var link step.LinkedDoc
		var metadataJSON sql.NullString
		var chunkID sql.NullInt64
		if err := rows.Scan(&docID, &link.Section, &link.Text, &link.Name, &metadataJSON, &chunkID); err != nil {
			return err
		}
		if !chunkID.Valid {
			continue // a document without chunks cannot be read
		}
		link.ChunkID = chunkID.Int64
		link.Metadata = map[string]any{}
		if metadataJSON.Valid {
			if err := json.Unmarshal([]byte(metadataJSON.String), &link.Metadata); err != nil {
				return fmt.Errorf("document %q: reading metadata_json: %w", link.Name, err)
			}
		}
		into[docID] = append(into[docID], link)
	}
	return rows.Err()
}

func (l *LinkLoader) querySections(ctx context.Context, in string, args []any, into map[int64][]step.SectionStart) (err error) {
	rows, err := l.db.QueryContext(ctx, `
		SELECT document_id, breadcrumb, id
		FROM chunks
		WHERE document_id IN `+in+`
		ORDER BY document_id, position, id`, args...)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var docID int64
		var s step.SectionStart
		if err := rows.Scan(&docID, &s.Breadcrumb, &s.ChunkID); err != nil {
			return err
		}
		into[docID] = append(into[docID], s)
	}
	return rows.Err()
}

func inClause(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return "(?" + strings.Repeat(", ?", len(ids)-1) + ")", args
}
