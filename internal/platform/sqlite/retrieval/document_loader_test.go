package retrieval

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"
)

func TestDocumentOfChunk(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	docID := sqlitetest.InsertDocumentNamed(t, db, "guide.md")
	if _, err := db.Exec(`UPDATE documents SET metadata_json = ? WHERE id = ?`, `{"enerchart_core": true}`, docID); err != nil {
		t.Fatalf("setting metadata_json: %v", err)
	}
	insert := func(position int64, breadcrumb, text string) int64 {
		t.Helper()
		res, err := db.Exec(`INSERT INTO chunks (created_at, document_id, position, type, text, breadcrumb) VALUES (?, ?, ?, ?, ?, ?)`,
			time.Now(), docID, position, "content", text, breadcrumb)
		if err != nil {
			t.Fatalf("inserting chunk: %v", err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	// inserted out of order loader must sort by position
	second := insert(1, "A > B", "A > B\n\nsecond")
	first := insert(0, "A", "A\n\nfirst")
	otherDocChunk := insertChunkInSource(t, db, "other doc", "manual", 1.0)

	loader := NewDocumentLoader(db, testLogger)

	t.Run("loads the document of the chunk with all its chunks in position order", func(t *testing.T) {
		got, err := loader.DocumentOfChunk(ctx, second, step.Filter{})
		if err != nil {
			t.Fatalf("DocumentOfChunk() error = %v", err)
		}
		want := step.LoadedDocument{
			Name:     "guide.md",
			Source:   sqlitetest.TestSource,
			Metadata: map[string]any{"enerchart_core": true},
			Chunks: []step.DocChunk{
				{ID: first, Position: 0, Breadcrumb: "A", Text: "A\n\nfirst"},
				{ID: second, Position: 1, Breadcrumb: "A > B", Text: "A > B\n\nsecond"},
			},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("document (-want +got):\n%s", diff)
		}
	})

	t.Run("a matching filter loads the document", func(t *testing.T) {
		filter := step.Filter{Sources: []string{sqlitetest.TestSource}, Metadata: map[string]any{"enerchart_core": true}}
		if _, err := loader.DocumentOfChunk(ctx, first, filter); err != nil {
			t.Fatalf("DocumentOfChunk() error = %v", err)
		}
	})

	t.Run("a filter that excludes the document gives ErrChunkNotInFilter", func(t *testing.T) {
		for _, filter := range []step.Filter{
			{Metadata: map[string]any{"enerchart_core": false}},
			{Sources: []string{"manual"}},
		} {
			if _, err := loader.DocumentOfChunk(ctx, first, filter); !errors.Is(err, step.ErrChunkNotInFilter) {
				t.Errorf("filter %+v: error = %v, want ErrChunkNotInFilter", filter, err)
			}
		}
	})

	t.Run("an unknown chunk gives ErrChunkNotFound", func(t *testing.T) {
		if _, err := loader.DocumentOfChunk(ctx, 999999, step.Filter{}); !errors.Is(err, step.ErrChunkNotFound) {
			t.Errorf("error = %v, want ErrChunkNotFound", err)
		}
	})

	t.Run("a document without metadata gives an empty map", func(t *testing.T) {
		got, err := loader.DocumentOfChunk(ctx, otherDocChunk, step.Filter{})
		if err != nil {
			t.Fatalf("DocumentOfChunk() error = %v", err)
		}
		if got.Metadata == nil || len(got.Metadata) != 0 || len(got.Chunks) != 1 {
			t.Errorf("document = %+v, want an empty metadata map and 1 chunk", got)
		}
	})
}
