package extraction

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"rag/internal/extract/step"
	"rag/internal/platform/config"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/sqlitetest"
)

var testCtx = context.Background()
var testLogger = slog.New(slog.DiscardHandler)

// newDBWithSources returns a test DB with the sources "manual" and "changelog".
// documents.source_name has a FK to sources, and the composition root writes them before the extraction.
func newDBWithSources(t *testing.T) *sql.DB {
	t.Helper()
	db := sqlitetest.New(t)
	sources := []config.Source{
		{Name: "manual", Weight: 0.8},
		{Name: "changelog", Weight: 0.4},
	}
	if err := sqlite.SaveSources(testCtx, db, sources); err != nil {
		t.Fatalf("SaveSources() error = %v", err)
	}
	return db
}

func simpleExtractedDoc(name, sha256, sourceName string) step.ExtractedDoc {
	return step.ExtractedDoc{
		MimeType: "application/pdf",
		Source: step.SourceDoc{
			Name:       name,
			RelPath:    "docs/" + name,
			SHA256:     sha256,
			SourceName: sourceName,
		},
		RootNodes: []*step.Node{
			{
				ID:      "n1",
				Kind:    step.KindHeading,
				Layer:   step.LayerBody,
				Heading: &step.HeadingContent{Text: "Title", Level: 1},
				Children: []*step.Node{
					{
						ID:        "n2",
						Kind:      step.KindParagraph,
						Layer:     step.LayerBody,
						Paragraph: &step.ParagraphContent{Text: "body text"},
					},
				},
			},
		},
	}
}

func TestSaveExtractedDoc(t *testing.T) {
	t.Run("happy path writes the document, extraction, and nodes", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		docID, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("manual.pdf", "sha-1", "manual"))
		if err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}
		if docID <= 0 {
			t.Fatalf("docID = %d, want a positive id", docID)
		}

		var name, relPath, sha, sourceName string
		if err := db.QueryRow("SELECT name, rel_path, sha256, source_name FROM documents WHERE id = ?", docID).Scan(&name, &relPath, &sha, &sourceName); err != nil {
			t.Fatalf("querying document: %v", err)
		}
		if name != "manual.pdf" || relPath != "docs/manual.pdf" || sha != "sha-1" || sourceName != "manual" {
			t.Errorf("document row = (%q, %q, %q, %q), want (%q, %q, %q, %q)", name, relPath, sha, sourceName, "manual.pdf", "docs/manual.pdf", "sha-1", "manual")
		}

		var mimeType string
		var extractionID int64
		if err := db.QueryRow("SELECT id, mime_type FROM extractions WHERE document_id = ?", docID).Scan(&extractionID, &mimeType); err != nil {
			t.Fatalf("querying extraction: %v", err)
		}
		if mimeType != "application/pdf" {
			t.Errorf("mime_type = %q, want application/pdf", mimeType)
		}

		var nodeCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM extraction_nodes WHERE extraction_id = ?", extractionID).Scan(&nodeCount); err != nil {
			t.Fatalf("counting extraction nodes: %v", err)
		}
		if nodeCount != 2 {
			t.Errorf("got %d extraction nodes, want 2", nodeCount)
		}

		var childParent sql.NullString
		if err := db.QueryRow("SELECT parent_id FROM extraction_nodes WHERE node_id = 'n2'").Scan(&childParent); err != nil {
			t.Fatalf("querying child node parent: %v", err)
		}
		if !childParent.Valid || childParent.String != "n1" {
			t.Errorf("n2's parent_id = %+v, want valid \"n1\"", childParent)
		}

		var rootParent sql.NullString
		if err := db.QueryRow("SELECT parent_id FROM extraction_nodes WHERE node_id = 'n1'").Scan(&rootParent); err != nil {
			t.Fatalf("querying root node parent: %v", err)
		}
		if rootParent.Valid {
			t.Errorf("n1's parent_id = %+v, want NULL (it's a root node)", rootParent)
		}
	})

	t.Run("a doc of an unknown source fails on the FK and writes nothing", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("x.pdf", "sha-1", "unknown")); err == nil {
			t.Fatalf("SaveExtractedDoc() error = nil, want a FOREIGN KEY constraint violation")
		}

		assertCount(t, db, "documents", 0)
		assertCount(t, db, "extractions", 0)
	})

	t.Run("the same rel path in the same source fails, the same rel path in another source is permitted", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("a.pdf", "sha-1", "manual")); err != nil {
			t.Fatalf("first SaveExtractedDoc() error = %v", err)
		}
		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("a.pdf", "sha-2", "manual")); err == nil {
			t.Fatalf("second SaveExtractedDoc() error = nil, want a UNIQUE(source_name, rel_path) violation")
		}
		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("a.pdf", "sha-3", "changelog")); err != nil {
			t.Fatalf("SaveExtractedDoc() in another source error = %v", err)
		}

		assertCount(t, db, "documents", 2)
		assertCount(t, db, "extractions", 2)
	})
}

func assertCount(t *testing.T, db *sql.DB, table string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&got); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	if got != want {
		t.Errorf("got %d rows in %s, want %d", got, table, want)
	}
}

func TestExistsDoc(t *testing.T) {
	t.Run("unknown sha256 reports not a duplicate", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		isDup, sourceName, err := sink.ExistsDoc(testCtx, "unknown-sha")
		if err != nil {
			t.Fatalf("ExistsDoc() error = %v", err)
		}
		if isDup || sourceName != "" {
			t.Errorf("ExistsDoc() = (%v, %q), want (false, \"\")", isDup, sourceName)
		}
	})

	t.Run("known sha256 reports the doc's actual source", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}
		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("manual.pdf", "sha-1", "manual")); err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}

		isDup, sourceName, err := sink.ExistsDoc(testCtx, "sha-1")
		if err != nil {
			t.Fatalf("ExistsDoc() error = %v", err)
		}
		if !isDup || sourceName != "manual" {
			t.Errorf("ExistsDoc() = (%v, %q), want (true, \"manual\")", isDup, sourceName)
		}
	})
}
