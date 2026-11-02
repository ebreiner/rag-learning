package serve

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"rag/internal/platform/sqlite/pool"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"
	"slices"
	"sync"

	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type poolDeps struct {
	Retrieval   step.RetrievalDeps
	Links       step.LinkDeps
	Read        step.ReadDeps
	QueryLogger *logging.QueryLogger
	Logger      *slog.Logger
}

type poolEntry struct {
	def       pool.Pool
	docs      int
	err       string
	resolving bool
	handler   http.Handler // nil when stopped, failed or resolving
}

// poolManager keeps the pools of the DB and serves /mcp/{name}, see knowledge/experiments/0005-pool-poc.md.
// opMu runs the changes one at a time: DB write and endpoint change stay in the same order.
// mu guards the map for the requests.
type poolManager struct {
	opMu    sync.Mutex
	mu      sync.RWMutex
	entries map[string]*poolEntry
	db      *sql.DB
	ro      *sql.DB
	deps    poolDeps
}

// newPoolManager loads the pools and resolves them in the background, so that slow pool queries
// do not delay the start of serve. Until then, an endpoint gives 503.
func newPoolManager(ctx context.Context, db, ro *sql.DB, deps poolDeps) (*poolManager, error) {
	m := &poolManager{entries: map[string]*poolEntry{}, db: db, ro: ro, deps: deps}
	pools, err := pool.List(ctx, db)
	if err != nil {
		return nil, err
	}
	m.opMu.Lock()
	for _, p := range pools {
		m.set(&poolEntry{def: p, resolving: p.State == pool.StateRunning})
	}
	go func() {
		defer m.opMu.Unlock()
		for _, p := range pools {
			m.activate(ctx, p)
		}
	}()
	return m, nil
}

func (m *poolManager) set(e *poolEntry) {
	m.mu.Lock()
	m.entries[e.def.Name] = e
	m.mu.Unlock()
}

func (m *poolManager) get(name string) (*poolEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	e, ok := m.entries[name]
	return e, ok
}

// activate resolves a running pool and installs its endpoint. A failed query does not stop serve:
// the entry keeps the error. The caller holds opMu. The context is not the one of a request,
// because a closed browser must not leave the pool broken.
func (m *poolManager) activate(ctx context.Context, p pool.Pool) *poolEntry {
	if p.State != pool.StateRunning {
		return m.install(p, nil, nil)
	}
	ids, err := pool.Resolve(context.WithoutCancel(ctx), m.ro, p.Query)
	return m.install(p, ids, err)
}

func (m *poolManager) install(p pool.Pool, ids []int64, err error) *poolEntry {
	entry := &poolEntry{def: p}
	switch {
	case err != nil:
		entry.err = err.Error()
		m.deps.Logger.Error("pool", "err", fmt.Errorf("pool %q: %w", p.Name, err))
	case p.State == pool.StateRunning:
		entry.docs = len(ids)
		entry.handler = m.mcpHandler(p, ids)
	}
	m.set(entry)
	return entry
}

func (m *poolManager) mcpHandler(p pool.Pool, ids []int64) http.Handler {
	filter := step.Filter{DocIDs: ids}
	s := server.NewMCPServer(
		"rag-"+p.Name,
		"0.0.1",
		server.WithLogging(),
		server.WithToolCapabilities(false),
		server.WithInstructions(p.Description),
	)
	rag := poolRetrievalTool(p.Description, p.Name, filter, m.deps.Retrieval, m.deps.Links, m.deps.QueryLogger)
	read := poolReadTool(p.Name, filter, m.deps.Read, m.deps.QueryLogger, m.deps.Logger)
	s.AddTool(rag.Tool, rag.Handler)
	s.AddTool(read.Tool, read.Handler)
	return server.NewStreamableHTTPServer(s)
}

func (m *poolManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	trace.SpanFromContext(r.Context()).SetAttributes(attribute.String("query.pool", name))
	entry, ok := m.get(name)
	switch {
	case !ok:
		http.Error(w, fmt.Sprintf("unknown pool %q", name), http.StatusNotFound)
	case entry.resolving:
		http.Error(w, fmt.Sprintf("pool %q is starting", name), http.StatusServiceUnavailable)
	case entry.handler == nil && entry.err != "":
		http.Error(w, fmt.Sprintf("pool %q has an error: %s", name, entry.err), http.StatusServiceUnavailable)
	case entry.handler == nil:
		http.Error(w, fmt.Sprintf("pool %q is stopped", name), http.StatusServiceUnavailable)
	default:
		entry.handler.ServeHTTP(w, r)
	}
}

type poolStatus struct {
	pool.Pool
	Documents int    `json:"documents"`
	Resolving bool   `json:"resolving,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (m *poolManager) list(ctx context.Context) ([]poolStatus, error) {
	pools, err := pool.List(ctx, m.db)
	if err != nil {
		return nil, err
	}
	out := make([]poolStatus, 0, len(pools))
	for _, p := range pools {
		st := poolStatus{Pool: p}
		if e, ok := m.get(p.Name); ok {
			st.Documents, st.Resolving, st.Error = e.docs, e.resolving, e.err
		}
		out = append(out, st)
	}
	return out, nil
}

// save creates or replaces a pool. A new pool starts as running, a changed pool keeps its state.
// A query that fails is not saved.
func (m *poolManager) save(ctx context.Context, p pool.Pool) (warning string, err error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	m.opMu.Lock()
	defer m.opMu.Unlock()

	ids, err := pool.Resolve(context.WithoutCancel(ctx), m.ro, p.Query)
	if err != nil {
		return "", err
	}
	p.State = pool.StateRunning
	if e, ok := m.get(p.Name); ok {
		p.State = e.def.State
	}
	if err := pool.Save(ctx, m.db, p); err != nil {
		return "", err
	}
	m.install(p, ids, nil)
	if len(ids) == 0 {
		warning = "the pool has no documents: its endpoint returns no results"
	}
	return warning, nil
}

func (m *poolManager) setState(ctx context.Context, name, state string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	e, ok := m.get(name)
	if !ok {
		return fmt.Errorf("%w: %q", pool.ErrNotFound, name)
	}
	p := e.def
	p.State = state
	if err := pool.Save(ctx, m.db, p); err != nil {
		return err
	}
	m.activate(ctx, p)
	return nil
}

func (m *poolManager) delete(ctx context.Context, name string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()

	if _, ok := m.get(name); !ok {
		return fmt.Errorf("%w: %q", pool.ErrNotFound, name)
	}
	if err := pool.Delete(ctx, m.db, name); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.entries, name)
	m.mu.Unlock()
	return nil
}

// importPools saves all pools as running in one step and resolves them in the background,
// because N slow queries would take longer than the WriteTimeout. A pool with a failed query
// stays saved and shows its error in the pool list.
func (m *poolManager) importPools(ctx context.Context, pools []pool.Pool) error {
	for _, p := range pools {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(pools))
	for _, p := range pools {
		if slices.Contains(names, p.Name) {
			return fmt.Errorf("%w: the file contains pool %q two times", pool.ErrInvalid, p.Name)
		}
		names = append(names, p.Name)
	}

	m.opMu.Lock()
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		m.opMu.Unlock()
		return err
	}
	for i := range pools {
		pools[i].State = pool.StateRunning
		if err := pool.Save(ctx, tx, pools[i]); err != nil {
			m.opMu.Unlock()
			return errors.Join(err, tx.Rollback())
		}
	}
	if err := tx.Commit(); err != nil {
		m.opMu.Unlock()
		return err
	}
	for _, p := range pools {
		m.set(&poolEntry{def: p, resolving: true})
	}
	go func() {
		defer m.opMu.Unlock()
		for _, p := range pools {
			m.activate(ctx, p)
		}
	}()
	return nil
}

func isUserErr(err error) bool {
	return errors.Is(err, pool.ErrInvalid)
}
