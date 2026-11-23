package extraction

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"

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

func TestSaveExtractedDocMetadata(t *testing.T) {
	t.Run("the metadata goes into metadata_json", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}
		doc := simpleExtractedDoc("a.pdf", "sha-1", "manual")
		doc.Source.Metadata = map[string]any{"classification.personal_data": true, "title": "A"}

		docID, err := sink.SaveExtractedDoc(testCtx, doc)
		if err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}

		var raw string
		if err := db.QueryRow("SELECT metadata_json FROM documents WHERE id = ?", docID).Scan(&raw); err != nil {
			t.Fatalf("querying metadata_json: %v", err)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("metadata_json is not JSON: %v", err)
		}
		if diff := cmp.Diff(doc.Source.Metadata, got); diff != "" {
			t.Errorf("metadata_json (-want +got):\n%s", diff)
		}
	})
}

func TestSaveExtractedDocLinks(t *testing.T) {
	type linkRow struct {
		Position  int64
		ToSource  string
		ToRelPath string
		Text      sql.NullString
		Section   string
	}
	readLinks := func(t *testing.T, db *sql.DB, docID int64) []linkRow {
		t.Helper()
		rows, err := db.Query(`SELECT position, to_source, to_rel_path, text, section FROM document_links WHERE from_document_id = ? ORDER BY position`, docID)
		if err != nil {
			t.Fatalf("querying document_links: %v", err)
		}
		defer func() {
			if err := rows.Close(); err != nil {
				t.Errorf("closing rows: %v", err)
			}
		}()
		var got []linkRow
		for rows.Next() {
			var r linkRow
			if err := rows.Scan(&r.Position, &r.ToSource, &r.ToRelPath, &r.Text, &r.Section); err != nil {
				t.Fatalf("scanning document_links: %v", err)
			}
			got = append(got, r)
		}
		return got
	}

	t.Run("the links are saved in order, with the source of the document", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}
		doc := simpleExtractedDoc("a.md", "sha-1", "manual")
		doc.Source.Links = []step.Link{{To: "b.md", Text: "B", Section: "S"}, {To: "dir/c.md"}}

		docID, err := sink.SaveExtractedDoc(testCtx, doc)
		if err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}

		want := []linkRow{
			{Position: 0, ToSource: "manual", ToRelPath: "b.md", Text: sql.NullString{String: "B", Valid: true}, Section: "S"},
			{Position: 1, ToSource: "manual", ToRelPath: "dir/c.md"},
		}
		if diff := cmp.Diff(want, readLinks(t, db, docID)); diff != "" {
			t.Errorf("document_links (-want +got):\n%s", diff)
		}
	})

	t.Run("a link error rolls back the whole document", func(t *testing.T) {
		db := newDBWithSources(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}
		doc := simpleExtractedDoc("a.md", "sha-1", "manual")
		// the step drops duplicates; here the UNIQUE constraint must catch one that passes it
		doc.Source.Links = []step.Link{{To: "b.md"}, {To: "b.md"}}

		if _, err := sink.SaveExtractedDoc(testCtx, doc); err == nil {
			t.Fatalf("SaveExtractedDoc() error = nil, want the UNIQUE error")
		}
		assertCount(t, db, "documents", 0)
		assertCount(t, db, "document_links", 0)
	})
}
