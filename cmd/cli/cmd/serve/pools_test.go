package serve

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/pool"
	"rag/internal/retrieval/step"
)

func newTestManager(t *testing.T) *poolManager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.NewConn(path, false)
	if err != nil {
		t.Fatalf("NewConn: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO sources (name, weight) VALUES ('manual', 1.0)`); err != nil {
		t.Fatalf("inserting source: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO documents (created_at, name, rel_path, sha256, source_name) VALUES (?, 'a.md', 'a.md', 'sha', 'manual')`, time.Now()); err != nil {
		t.Fatalf("inserting document: %v", err)
	}
	ro, err := pool.NewReadOnlyConn(path)
	if err != nil {
		t.Fatalf("NewReadOnlyConn: %v", err)
	}
	t.Cleanup(func() { _ = ro.Close() })

	logger := slog.New(slog.DiscardHandler)
	m, err := newPoolManager(context.Background(), db, ro, poolDeps{
		Retrieval:   step.RetrievalDeps{Logger: logger},
		QueryLogger: discardQueryLogger(t),
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("newPoolManager: %v", err)
	}
	return m
}

// waitResolved waits for the background resolution of serve start and import.
func waitResolved(t *testing.T, m *poolManager) {
	t.Helper()
	m.opMu.Lock()
	defer m.opMu.Unlock()
}

func status(t *testing.T, m *poolManager, name string) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp/"+name, nil)
	req.SetPathValue("name", name)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)
	return rec.Code
}

func TestPoolManager(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	p := pool.Pool{Name: "docs", Description: "d", Query: `SELECT id FROM pool_documents`}

	if _, err := m.save(ctx, pool.Pool{Name: "bad", Description: "d", Query: `SELECT 'x'`}); !errors.Is(err, pool.ErrInvalid) {
		t.Errorf("save of a bad query: error = %v, want ErrInvalid", err)
	}
	if _, err := m.save(ctx, p); err != nil {
		t.Fatalf("save: %v", err)
	}
	if code := status(t, m, "docs"); code == http.StatusNotFound || code == http.StatusServiceUnavailable {
		t.Errorf("running pool: status %d, want the MCP handler", code)
	}

	if err := m.setState(ctx, "docs", pool.StateStopped); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if code := status(t, m, "docs"); code != http.StatusServiceUnavailable {
		t.Errorf("stopped pool: status %d, want 503", code)
	}
	if _, err := m.save(ctx, p); err != nil {
		t.Fatalf("save of a stopped pool: %v", err)
	}
	if e, _ := m.get("docs"); e.def.State != pool.StateStopped {
		t.Errorf("a changed pool has state %q, want it to keep stopped", e.def.State)
	}

	for name, err := range map[string]error{
		"start":  m.setState(ctx, "nope", pool.StateRunning),
		"stop":   m.setState(ctx, "nope", pool.StateStopped),
		"delete": m.delete(ctx, "nope"),
	} {
		if !errors.Is(err, pool.ErrNotFound) {
			t.Errorf("%s of an unknown pool: error = %v, want ErrNotFound", name, err)
		}
	}

	if err := m.delete(ctx, "docs"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if code := status(t, m, "docs"); code != http.StatusNotFound {
		t.Errorf("deleted pool: status %d, want 404", code)
	}

	dup := []pool.Pool{p, p}
	if err := m.importPools(ctx, dup); !errors.Is(err, pool.ErrInvalid) {
		t.Errorf("import with a name two times: error = %v, want ErrInvalid", err)
	}
	broken := pool.Pool{Name: "broken", Description: "d", Query: `SELECT id FROM no_such_table`}
	if err := m.importPools(ctx, []pool.Pool{p, broken}); err != nil {
		t.Fatalf("import: %v", err)
	}
	waitResolved(t, m)
	list, err := m.list(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("list = %+v, %v, want 2 pools", list, err)
	}
	for _, st := range list {
		if st.State != pool.StateRunning || st.Resolving {
			t.Errorf("pool %q: state %q, resolving %v, want running and resolved", st.Name, st.State, st.Resolving)
		}
	}
	if code := status(t, m, "broken"); code != http.StatusServiceUnavailable {
		t.Errorf("pool with a failed query: status %d, want 503", code)
	}
}

func TestPoolManagerStart(t *testing.T) {
	ctx := context.Background()
	m := newTestManager(t)
	if _, err := m.save(ctx, pool.Pool{Name: "docs", Description: "d", Query: `SELECT id FROM pool_documents`}); err != nil {
		t.Fatalf("save: %v", err)
	}

	again, err := newPoolManager(ctx, m.db, m.ro, m.deps)
	if err != nil {
		t.Fatalf("newPoolManager: %v", err)
	}
	waitResolved(t, again)
	if e, ok := again.get("docs"); !ok || e.handler == nil || e.docs != 1 {
		t.Errorf("after a restart: entry %+v, want a running pool with 1 document", e)
	}
}
