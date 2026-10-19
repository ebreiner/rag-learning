package retrieval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"rag/internal/retrieval/step"
)

type DocumentLoader struct {
	db     *sql.DB
	Logger *slog.Logger
}

func NewDocumentLoader(db *sql.DB, logger *slog.Logger) DocumentLoader {
	return DocumentLoader{db: db, Logger: logger}
}

func (l *DocumentLoader) DocumentOfChunk(ctx context.Context, chunkID int64, filter step.Filter) (doc step.LoadedDocument, err error) {
	clause, filterArgs := filterClause(filter)
	var docID int64
	var metadataJSON sql.NullString
	err = l.db.QueryRowContext(ctx, `
		SELECT d.id, d.name, d.source_name, d.metadata_json
		FROM chunks AS c
		JOIN documents AS d ON d.id = c.document_id
		WHERE c.id = ?`+clause, append([]any{chunkID}, filterArgs...)...).Scan(&docID, &doc.Name, &doc.Source, &metadataJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return doc, l.missingChunkErr(ctx, chunkID)
	}
	if err != nil {
		return doc, fmt.Errorf("reading the document of chunk %d: %w", chunkID, err)
	}

	doc.Metadata = map[string]any{}
	if metadataJSON.Valid {
		if err := json.Unmarshal([]byte(metadataJSON.String), &doc.Metadata); err != nil {
			return doc, fmt.Errorf("document %d: reading metadata_json: %w", docID, err)
		}
	}

	rows, err := l.db.QueryContext(ctx, `
		SELECT id, position, breadcrumb, text
		FROM chunks
		WHERE document_id = ?
		ORDER BY position, id`, docID)
	if err != nil {
		return doc, fmt.Errorf("reading the chunks of document %d: %w", docID, err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var c step.DocChunk
		if err := rows.Scan(&c.ID, &c.Position, &c.Breadcrumb, &c.Text); err != nil {
			return doc, err
		}
		doc.Chunks = append(doc.Chunks, c)
	}
	return doc, rows.Err()
}

func (l *DocumentLoader) missingChunkErr(ctx context.Context, chunkID int64) error {
	var exists bool
	if err := l.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM chunks WHERE id = ?)`, chunkID).Scan(&exists); err != nil {
		return fmt.Errorf("checking chunk %d: %w", chunkID, err)
	}
	if exists {
		return fmt.Errorf("%w: chunk %d", step.ErrChunkNotInFilter, chunkID)
	}
	return fmt.Errorf("%w: chunk %d", step.ErrChunkNotFound, chunkID)
}
