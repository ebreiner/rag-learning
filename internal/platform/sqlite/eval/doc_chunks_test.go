package eval

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"
	"time"

	"rag/internal/eval/step"
	"rag/internal/platform/sqlite/sqlitetest"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var (
	testCtx    = context.Background()
	testLogger = slog.New(slog.DiscardHandler)
)

func insertChunk(t *testing.T, db *sql.DB, docID int64, text, breadcrumb string) int64 {
	t.Helper()
	res, err := db.ExecContext(testCtx,
		`INSERT INTO chunks (created_at, document_id, position, type, text, breadcrumb) VALUES (?, ?, ?, ?, ?, ?)`,
		time.Now(), docID, 0, "content", text, breadcrumb,
	)
	if err != nil {
		t.Fatalf("inserting chunk: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("chunk id: %v", err)
	}
	return id
}

func TestChunksOfDoc(t *testing.T) {
	db := sqlitetest.New(t)

	// two documents with one name: the "changed file" case before re-index exists
	docA1 := sqlitetest.InsertDocumentNamed(t, db, "a.md")
	docA2 := sqlitetest.InsertDocumentNamed(t, db, "a.md")
	sqlitetest.InsertDocumentNamed(t, db, "empty.md")
	docB := sqlitetest.InsertDocumentNamed(t, db, "b.md")

	c1 := insertChunk(t, db, docA1, "first", "A > One")
	c2 := insertChunk(t, db, docA2, "second", "A > Two")
	insertChunk(t, db, docB, "other doc", "B")

	p := NewDocChunksProvider(db, testLogger)

	t.Run("all chunks of all documents with the name", func(t *testing.T) {
		got, exists, err := p.ChunksOfDoc(testCtx, "a.md")
		if err != nil {
			t.Fatalf("ChunksOfDoc() error = %v", err)
		}
		if !exists {
			t.Fatal("exists = false, want true")
		}
		want := []step.Chunk{
			{ID: c1, DocName: "a.md", Breadcrumb: "A > One", Text: "first"},
			{ID: c2, DocName: "a.md", Breadcrumb: "A > Two", Text: "second"},
		}
		sortByID := cmpopts.SortSlices(func(a, b step.Chunk) bool { return a.ID < b.ID })
		if diff := cmp.Diff(want, got, sortByID); diff != "" {
			t.Errorf("ChunksOfDoc() (-want +got):\n%s", diff)
		}
	})

	t.Run("document exists without chunks", func(t *testing.T) {
		got, exists, err := p.ChunksOfDoc(testCtx, "empty.md")
		if err != nil {
			t.Fatalf("ChunksOfDoc() error = %v", err)
		}
		if !exists || len(got) != 0 {
			t.Errorf("ChunksOfDoc() = %d chunks, exists %v; want 0 chunks, exists true", len(got), exists)
		}
	})

	t.Run("document does not exist", func(t *testing.T) {
		got, exists, err := p.ChunksOfDoc(testCtx, "nope.md")
		if err != nil {
			t.Fatalf("ChunksOfDoc() error = %v", err)
		}
		if exists || got != nil {
			t.Errorf("ChunksOfDoc() = %v, exists %v; want nil, exists false", got, exists)
		}
	})
}
