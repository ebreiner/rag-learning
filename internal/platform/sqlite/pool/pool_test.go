package pool

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"rag/internal/platform/sqlite"
)

// newDBs gives a read-write DB with two documents in source "manual" and one in "changelogs",
// and a read-only connection to the same file.
func newDBs(t *testing.T) (rw, ro *sql.DB, ids []int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	rw, err := sqlite.NewConn(path, false)
	if err != nil {
		t.Fatalf("NewConn: %v", err)
	}
	t.Cleanup(func() { _ = rw.Close() })
	for _, src := range []string{"manual", "changelogs"} {
		if _, err := rw.Exec(`INSERT INTO sources (name, weight) VALUES (?, 1.0)`, src); err != nil {
			t.Fatalf("inserting source: %v", err)
		}
	}
	for i, doc := range []struct{ source, path, metadata string }{
		{"manual", "a.md", `{"doc.title": "A", "doc.freshness": "current", "doc.url": "https://example.org/a"}`},
		{"manual", "b.md", `{"doc.freshness": "outdated"}`},
		{"changelogs", "c.md", `{}`},
	} {
		res, err := rw.Exec(`INSERT INTO documents (created_at, name, rel_path, sha256, metadata_json, source_name) VALUES (?, ?, ?, ?, ?, ?)`,
			time.Now(), doc.path, doc.path, "sha"+string(rune('0'+i)), doc.metadata, doc.source)
		if err != nil {
			t.Fatalf("inserting document: %v", err)
		}
		id, _ := res.LastInsertId()
		ids = append(ids, id)
	}
	ro, err = NewReadOnlyConn(path)
	if err != nil {
		t.Fatalf("NewReadOnlyConn: %v", err)
	}
	t.Cleanup(func() { _ = ro.Close() })
	return rw, ro, ids
}

func TestResolve(t *testing.T) {
	_, ro, ids := newDBs(t)
	ctx := context.Background()

	ok := []struct {
		name  string
		query string
		want  []int64
	}{
		{"by source", `SELECT id FROM pool_documents WHERE source_name = 'manual'`, ids[:2]},
		{"by metadata with a dotted key", `SELECT id FROM pool_documents WHERE json_extract(metadata, '$."doc.freshness"') = 'current'`, ids[:1]},
		{"a trailing semicolon", "SELECT id FROM pool_documents WHERE rel_path = 'c.md';\n", ids[2:]},
		{"a trailing comment", "SELECT id FROM pool_documents WHERE rel_path = 'c.md' -- the changelog", ids[2:]},
		{"a CTE", `WITH m AS (SELECT id FROM pool_documents WHERE source_name = 'changelogs') SELECT id FROM m`, ids[2:]},
		{"duplicates count once", `SELECT id FROM pool_documents UNION ALL SELECT id FROM pool_documents`, ids},
		{"no rows give an empty, not a nil list", `SELECT id FROM pool_documents WHERE 0`, []int64{}},
	}
	for _, tt := range ok {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(ctx, ro, tt.query)
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if got == nil || !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	bad := []struct {
		name  string
		query string
	}{
		{"two columns", `SELECT id, title FROM pool_documents`},
		{"text values", `SELECT rel_path FROM pool_documents`},
		{"an ID that is not a document", `SELECT 999999`},
		{"a second statement", `SELECT id FROM pool_documents; SELECT 1`},
		{"a write", `DELETE FROM documents`},
		{"an attach", `ATTACH DATABASE 'x.db' AS x`},
		{"a pragma", `PRAGMA query_only = 0`},
		{"a syntax error", `SELEC id`},
	}
	for _, tt := range bad {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Resolve(ctx, ro, tt.query); !errors.Is(err, ErrInvalid) {
				t.Errorf("Resolve() error = %v, want ErrInvalid", err)
			}
		})
	}
}

// TestSandbox runs the attacks of the security review on one connection, so that a state
// such as an ATTACH would stay for the next statement.
func TestSandbox(t *testing.T) {
	rw, ro, _ := newDBs(t)
	ctx := context.Background()
	ro.SetMaxOpenConns(1)
	if err := Save(ctx, rw, Pool{Name: "keep", Description: "d", Query: "SELECT 1", State: StateRunning}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	var path string
	if err := rw.QueryRow(`SELECT file FROM pragma_database_list WHERE name = 'main'`).Scan(&path); err != nil {
		t.Fatalf("reading the db path: %v", err)
	}

	for _, attack := range []string{
		"SELECT 1); ATTACH 'file:" + path + "?mode=rw' AS (w",
		"SELECT 1); ATTACH '" + filepath.Join(t.TempDir(), "new.db") + "' AS (y",
		"SELECT 1); PRAGMA query_only(0",
		"SELECT 1); DELETE FROM pools WHERE (1",
		"SELECT 1); DELETE FROM w.pools WHERE (1",
		"SELECT 1); PRAGMA hard_heap_limit(100000",
		"SELECT * FROM pragma_soft_heap_limit(1)",
		"SELECT 1); VACUUM INTO '" + filepath.Join(t.TempDir(), "v.db") + "' --",
		"SELECT load_extension('x')",
		"SELECT randomblob(300000)",
	} {
		if _, err := Run(ctx, ro, attack); !errors.Is(err, ErrInvalid) {
			t.Errorf("Run(%q) error = %v, want ErrInvalid", attack, err)
		}
	}

	var n int
	if err := rw.QueryRow(`SELECT COUNT(*) FROM pools`).Scan(&n); err != nil || n != 1 {
		t.Errorf("pools after the attacks: %d rows, err %v, want 1 row", n, err)
	}
	if _, err := Run(ctx, ro, `SELECT id FROM pool_documents`); err != nil {
		t.Errorf("a normal query after the attacks: %v", err)
	}
}

func TestReadOnlyConnReadsVirtualTables(t *testing.T) {
	rw, ro, _ := newDBs(t)
	ctx := context.Background()
	table, err := sqlite.SetupVecTable(ctx, rw, 3, "bge-m3")
	if err != nil {
		t.Fatalf("SetupVecTable: %v", err)
	}
	for _, q := range []string{
		`SELECT rowid FROM chunks_fts WHERE chunks_fts MATCH 'anchor'`,
		`SELECT chunk_id FROM ` + table,
		`SELECT value FROM json_each('[1, 2]')`,
	} {
		if _, err := Run(ctx, ro, q); err != nil {
			t.Errorf("Run(%q) error = %v", q, err)
		}
	}
}

func TestRun(t *testing.T) {
	_, ro, _ := newDBs(t)
	ctx := context.Background()

	res, err := Run(ctx, ro, `SELECT source_name, COUNT(*) AS n FROM pool_documents GROUP BY source_name ORDER BY source_name`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !slices.Equal(res.Columns, []string{"source_name", "n"}) || len(res.Rows) != 2 || res.Rows[0][0] != "changelogs" {
		t.Errorf("got %+v", res)
	}

	res, err = Run(ctx, ro, `WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < 600) SELECT i FROM n`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(res.Rows) != maxRows || !res.Truncated {
		t.Errorf("got %d rows, truncated %v, want %d rows, truncated", len(res.Rows), res.Truncated, maxRows)
	}
}

func TestRunDocumentRefs(t *testing.T) {
	_, ro, ids := newDBs(t)
	ctx := context.Background()

	res, err := Run(ctx, ro, `SELECT id FROM pool_documents WHERE source_name = 'manual'`)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := map[int64]DocumentRef{
		ids[0]: {SourceName: "manual", Title: "A", URL: "https://example.org/a"},
		ids[1]: {SourceName: "manual", Title: "b.md"},
	}
	if diff := cmp.Diff(want, res.Documents); diff != "" {
		t.Errorf("documents (-want +got), the title falls back to the name:\n%s", diff)
	}

	for _, q := range []string{`SELECT id, title FROM pool_documents`, `SELECT rel_path FROM pool_documents`, `SELECT id FROM pool_documents WHERE 0`} {
		res, err := Run(ctx, ro, q)
		if err != nil || res.Documents != nil {
			t.Errorf("Run(%q) = documents %v, err %v, want no documents", q, res.Documents, err)
		}
	}
}

func TestStore(t *testing.T) {
	rw, _, _ := newDBs(t)
	ctx := context.Background()

	p := Pool{Name: "ops", Description: "d", Query: "SELECT 1", State: StateRunning}
	if err := Save(ctx, rw, p); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	p.State = StateStopped
	if err := Save(ctx, rw, p); err != nil {
		t.Fatalf("Save() again error = %v", err)
	}
	got, err := List(ctx, rw)
	if err != nil || len(got) != 1 || got[0] != p {
		t.Fatalf("List() = %+v, %v, want [%+v]", got, err, p)
	}
	if err := Delete(ctx, rw, "ops"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if got, _ := List(ctx, rw); len(got) != 0 {
		t.Errorf("List() after delete = %+v", got)
	}
}

func TestValidate(t *testing.T) {
	for _, p := range []Pool{
		{Name: "Ops", Description: "d", Query: "q"},
		{Name: "ui", Description: "d", Query: "q"},
		{Name: "a/b", Description: "d", Query: "q"},
		{Name: "ops", Description: " ", Query: "q"},
		{Name: "ops", Description: "d", Query: ""},
	} {
		if err := p.Validate(); !errors.Is(err, ErrInvalid) {
			t.Errorf("Validate(%+v) = %v, want ErrInvalid", p, err)
		}
	}
	if err := (Pool{Name: "ops-2", Description: "d", Query: "q"}).Validate(); err != nil {
		t.Errorf("Validate() = %v", err)
	}
}
