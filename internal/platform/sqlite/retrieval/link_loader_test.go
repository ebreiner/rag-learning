package retrieval

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"

	"github.com/google/go-cmp/cmp"
)

type linkFixture struct {
	db    *sql.DB
	docID map[string]int64
	first map[string]int64 // the first chunk of each document
}

func (f *linkFixture) insertLinkDoc(t *testing.T, source, relPath, metadataJSON string, breadcrumbs ...string) {
	t.Helper()
	now := time.Now()
	if _, err := f.db.Exec(`INSERT INTO sources (name, weight) VALUES (?, 1.0) ON CONFLICT(name) DO NOTHING`, source); err != nil {
		t.Fatalf("inserting source: %v", err)
	}
	var metadata any
	if metadataJSON != "" {
		metadata = metadataJSON
	}
	res, err := f.db.Exec(`INSERT INTO documents (created_at, name, rel_path, sha256, metadata_json, source_name) VALUES (?, ?, ?, ?, ?, ?)`,
		now, relPath, relPath, "sha-"+source+"-"+relPath, metadata, source)
	if err != nil {
		t.Fatalf("inserting document: %v", err)
	}
	docID, _ := res.LastInsertId()
	f.docID[source+"/"+relPath] = docID
	// insert in reverse order, so that the chunk ID order differs from the position order
	for pos := len(breadcrumbs) - 1; pos >= 0; pos-- {
		res, err := f.db.Exec(`INSERT INTO chunks (created_at, document_id, position, type, text, breadcrumb) VALUES (?, ?, ?, 'content', 'text', ?)`,
			now, docID, pos, breadcrumbs[pos])
		if err != nil {
			t.Fatalf("inserting chunk: %v", err)
		}
		if pos == 0 {
			f.first[source+"/"+relPath], _ = res.LastInsertId()
		}
	}
}

func (f *linkFixture) link(t *testing.T, from string, position int, toSource, toRelPath, text, section string) {
	t.Helper()
	var textArg any
	if text != "" {
		textArg = text
	}
	if _, err := f.db.Exec(`INSERT INTO document_links (from_document_id, position, to_source, to_rel_path, text, section) VALUES (?, ?, ?, ?, ?, ?)`,
		f.docID["wiki/"+from], position, toSource, toRelPath, textArg, section); err != nil {
		t.Fatalf("inserting link: %v", err)
	}
}

func TestLoadLinks(t *testing.T) {
	ctx := context.Background()
	f := &linkFixture{db: sqlitetest.New(t), docID: map[string]int64{}, first: map[string]int64{}}
	f.insertLinkDoc(t, "wiki", "hub.md", `{"doc.title": "Hub", "personal": false}`, "Hub", "Hub > Admin", "Hub > Docs")
	f.insertLinkDoc(t, "wiki", "b.md", `{"doc.title": "B", "personal": false}`, "B")
	f.insertLinkDoc(t, "wiki", "c.md", `{"personal": true}`, "C")
	f.insertLinkDoc(t, "wiki", "empty.md", `{"personal": false}`) // no chunks
	f.insertLinkDoc(t, "manual", "b.md", `{"personal": false}`, "other source")

	f.link(t, "hub.md", 0, "wiki", "c.md", "", "Docs")
	f.link(t, "hub.md", 1, "wiki", "b.md", "see B", "Admin")
	f.link(t, "hub.md", 2, "wiki", "missing.md", "", "Admin")
	f.link(t, "hub.md", 3, "wiki", "empty.md", "", "Admin")
	f.link(t, "c.md", 0, "wiki", "b.md", "", "")

	loader := NewLinkLoader(f.db, testLogger)

	t.Run("outgoing and incoming links, targets resolved by source and rel_path", func(t *testing.T) {
		got, err := loader.LoadLinks(ctx, []int64{f.docID["wiki/hub.md"], f.docID["wiki/b.md"]}, step.Filter{})
		if err != nil {
			t.Fatalf("LoadLinks() error = %v", err)
		}
		wantOut := map[int64][]step.LinkedDoc{
			f.docID["wiki/hub.md"]: {
				{Section: "Docs", Name: "c.md", Metadata: map[string]any{"personal": true}, ChunkID: f.first["wiki/c.md"]},
				{Section: "Admin", Text: "see B", Name: "b.md", Metadata: map[string]any{"doc.title": "B", "personal": false}, ChunkID: f.first["wiki/b.md"]},
			},
		}
		if diff := cmp.Diff(wantOut, got.Out); diff != "" {
			t.Errorf("out (-want +got), a missing target and a target without chunks must not appear:\n%s", diff)
		}
		wantIn := map[int64][]step.LinkedDoc{
			f.docID["wiki/b.md"]: {
				{Section: "Admin", Text: "see B", Name: "hub.md", Metadata: map[string]any{"doc.title": "Hub", "personal": false}, ChunkID: f.first["wiki/hub.md"]},
				{Section: "", Name: "c.md", Metadata: map[string]any{"personal": true}, ChunkID: f.first["wiki/c.md"]},
			},
		}
		if diff := cmp.Diff(wantIn, got.In); diff != "" {
			t.Errorf("in (-want +got):\n%s", diff)
		}
		wantSections := []step.SectionStart{
			{Breadcrumb: "Hub", ChunkID: f.first["wiki/hub.md"]},
			{Breadcrumb: "Hub > Admin", ChunkID: f.first["wiki/hub.md"] - 1},
			{Breadcrumb: "Hub > Docs", ChunkID: f.first["wiki/hub.md"] - 2},
		}
		if diff := cmp.Diff(wantSections, got.Sections[f.docID["wiki/hub.md"]]); diff != "" {
			t.Errorf("sections of hub.md (-want +got), position order:\n%s", diff)
		}
	})

	t.Run("the filter applies to the other document of each link", func(t *testing.T) {
		got, err := loader.LoadLinks(ctx, []int64{f.docID["wiki/hub.md"], f.docID["wiki/b.md"]}, step.Filter{Metadata: map[string]any{"personal": false}})
		if err != nil {
			t.Fatalf("LoadLinks() error = %v", err)
		}
		for _, l := range got.Out[f.docID["wiki/hub.md"]] {
			if l.Name == "c.md" {
				t.Errorf("out contains c.md, which is outside the filter")
			}
		}
		if n := len(got.Out[f.docID["wiki/hub.md"]]); n != 1 {
			t.Errorf("out of hub.md = %d links, want 1 (b.md)", n)
		}
		if n := len(got.In[f.docID["wiki/b.md"]]); n != 1 || got.In[f.docID["wiki/b.md"]][0].Name != "hub.md" {
			t.Errorf("in of b.md = %+v, want only hub.md", got.In[f.docID["wiki/b.md"]])
		}
	})

	t.Run("a source filter also applies to the other document", func(t *testing.T) {
		got, err := loader.LoadLinks(ctx, []int64{f.docID["wiki/hub.md"]}, step.Filter{Sources: []string{"manual"}})
		if err != nil {
			t.Fatalf("LoadLinks() error = %v", err)
		}
		if n := len(got.Out[f.docID["wiki/hub.md"]]); n != 0 {
			t.Errorf("out = %d links, want 0: the targets are in wiki, the filter is manual", n)
		}
	})

	t.Run("no document IDs give empty maps", func(t *testing.T) {
		got, err := loader.LoadLinks(ctx, nil, step.Filter{})
		if err != nil {
			t.Fatalf("LoadLinks() error = %v", err)
		}
		if len(got.Out) != 0 || len(got.In) != 0 || len(got.Sections) != 0 {
			t.Errorf("got %+v, want empty maps", got)
		}
	})
}
