package extraction

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"rag/internal/extract/step"
	"rag/internal/platform/sqlite/sqlitetest"
)

var testCtx = context.Background()
var testLogger = slog.New(slog.DiscardHandler)

func simpleExtractedDoc(name, sha256, collectionName string, weight float64) step.ExtractedDoc {
	return step.ExtractedDoc{
		MimeType: "application/pdf",
		Source: step.SourceDoc{
			Name:             name,
			SHA256:           sha256,
			CollectionName:   collectionName,
			CollectionWeight: weight,
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
	t.Run("happy path writes the collection, document, extraction, and nodes", func(t *testing.T) {
		db := sqlitetest.New(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		docID, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("manual.pdf", "sha-1", "manual", 0.8))
		if err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}
		if docID <= 0 {
			t.Fatalf("docID = %d, want a positive id", docID)
		}

		var name, sha, collectionName string
		if err := db.QueryRow("SELECT name, sha256, collection_name FROM documents WHERE id = ?", docID).Scan(&name, &sha, &collectionName); err != nil {
			t.Fatalf("querying document: %v", err)
		}
		if name != "manual.pdf" || sha != "sha-1" || collectionName != "manual" {
			t.Errorf("document row = (%q, %q, %q), want (%q, %q, %q)", name, sha, collectionName, "manual.pdf", "sha-1", "manual")
		}

		var weight float64
		if err := db.QueryRow("SELECT weight FROM collections WHERE name = 'manual'").Scan(&weight); err != nil {
			t.Fatalf("querying collection weight: %v", err)
		}
		if weight != 0.8 {
			t.Errorf("collection weight = %v, want 0.8", weight)
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

	t.Run("saving a second doc under the same collection with a new weight updates the collection live", func(t *testing.T) {
		db := sqlitetest.New(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("first.pdf", "sha-1", "manual", 0.8)); err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}
		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("second.pdf", "sha-2", "manual", 0.3)); err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}

		var weight float64
		if err := db.QueryRow("SELECT weight FROM collections WHERE name = 'manual'").Scan(&weight); err != nil {
			t.Fatalf("querying collection weight: %v", err)
		}
		if weight != 0.3 {
			t.Errorf("collection weight = %v, want 0.3 (the most recently saved value)", weight)
		}
	})

	t.Run("a failure partway through rolls back the whole write, leaving prior state untouched", func(t *testing.T) {
		db := sqlitetest.New(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("first.pdf", "sha-1", "manual", 1.0)); err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}

		// weight < 0 violates the collections.weight CHECK constraint --
		// the whole transaction, including the document insert, must roll back.
		_, err = sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("second.pdf", "sha-2", "manual", -1.0))
		if err == nil {
			t.Fatalf("SaveExtractedDoc() error = nil, want a CHECK constraint violation")
		}

		var docCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM documents").Scan(&docCount); err != nil {
			t.Fatalf("counting documents: %v", err)
		}
		if docCount != 1 {
			t.Errorf("got %d documents, want 1 (the failed second save must not have partially committed)", docCount)
		}

		var weight float64
		if err := db.QueryRow("SELECT weight FROM collections WHERE name = 'manual'").Scan(&weight); err != nil {
			t.Fatalf("querying collection weight: %v", err)
		}
		if weight != 1.0 {
			t.Errorf("collection weight = %v, want 1.0 (unchanged by the rolled-back save)", weight)
		}
	})
}

func TestExistsDoc(t *testing.T) {
	t.Run("unknown sha256 reports not a duplicate", func(t *testing.T) {
		db := sqlitetest.New(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}

		isDup, collectionName, err := sink.ExistsDoc(testCtx, "unknown-sha")
		if err != nil {
			t.Fatalf("ExistsDoc() error = %v", err)
		}
		if isDup || collectionName != "" {
			t.Errorf("ExistsDoc() = (%v, %q), want (false, \"\")", isDup, collectionName)
		}
	})

	t.Run("known sha256 reports the doc's actual collection", func(t *testing.T) {
		db := sqlitetest.New(t)
		sink, err := NewExtractedDocSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewExtractedDocSink() error = %v", err)
		}
		if _, err := sink.SaveExtractedDoc(testCtx, simpleExtractedDoc("manual.pdf", "sha-1", "manual", 0.8)); err != nil {
			t.Fatalf("SaveExtractedDoc() error = %v", err)
		}

		isDup, collectionName, err := sink.ExistsDoc(testCtx, "sha-1")
		if err != nil {
			t.Fatalf("ExistsDoc() error = %v", err)
		}
		if !isDup || collectionName != "manual" {
			t.Errorf("ExistsDoc() = (%v, %q), want (true, \"manual\")", isDup, collectionName)
		}
	})
}
