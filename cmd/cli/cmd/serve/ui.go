package serve

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"rag/internal/platform/sqlite/pool"
)

//go:embed ui/index.html
var uiPage []byte

const exportVersion = 1

type poolFile struct {
	Version int         `json:"version"`
	Pools   []pool.Pool `json:"pools"`
}

// uiHandler serves the page without the token and the API with the token.
func uiHandler(m *poolManager, auth func(http.Handler) http.Handler) http.Handler {
	api := http.NewServeMux()
	api.HandleFunc("GET /ui/api/documents", m.handleDocuments)
	api.HandleFunc("POST /ui/api/query", m.handleQuery)
	api.HandleFunc("GET /ui/api/pools", m.handleList)
	api.HandleFunc("POST /ui/api/pools", m.handleSave)
	api.HandleFunc("POST /ui/api/pools/{name}/start", m.handleState(pool.StateRunning))
	api.HandleFunc("POST /ui/api/pools/{name}/stop", m.handleState(pool.StateStopped))
	api.HandleFunc("DELETE /ui/api/pools/{name}", m.handleDelete)
	api.HandleFunc("GET /ui/api/export", m.handleExport)
	api.HandleFunc("POST /ui/api/import", m.handleImport)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ui/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(uiPage)
	})
	mux.Handle("/ui/api/", auth(api))
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (m *poolManager) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pool.ErrNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	if isUserErr(err) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	m.deps.Logger.ErrorContext(r.Context(), "ui", "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

func (m *poolManager) handleDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := pool.ListDocuments(r.Context(), m.ro)
	if err != nil {
		m.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, docs)
}

func (m *poolManager) handleQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		m.writeErr(w, r, fmt.Errorf("%w: %w", pool.ErrInvalid, err))
		return
	}
	res, err := pool.Run(r.Context(), m.ro, body.Query)
	if err != nil {
		m.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (m *poolManager) handleList(w http.ResponseWriter, r *http.Request) {
	pools, err := m.list(r.Context())
	if err != nil {
		m.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, pools)
}

func (m *poolManager) handleSave(w http.ResponseWriter, r *http.Request) {
	var p pool.Pool
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		m.writeErr(w, r, fmt.Errorf("%w: %w", pool.ErrInvalid, err))
		return
	}
	warning, err := m.save(r.Context(), p)
	if err != nil {
		m.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"warning": warning})
}

func (m *poolManager) handleState(state string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := m.setState(r.Context(), r.PathValue("name"), state); err != nil {
			m.writeErr(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{})
	}
}

func (m *poolManager) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := m.delete(r.Context(), r.PathValue("name")); err != nil {
		m.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// handleExport gives all pools without their state: an import starts each pool.
func (m *poolManager) handleExport(w http.ResponseWriter, r *http.Request) {
	pools, err := pool.List(r.Context(), m.db)
	if err != nil {
		m.writeErr(w, r, err)
		return
	}
	file := poolFile{Version: exportVersion, Pools: []pool.Pool{}}
	for _, p := range pools {
		p.State = ""
		file.Pools = append(file.Pools, p)
	}
	w.Header().Set("Content-Disposition", `attachment; filename="pools.json"`)
	writeJSON(w, http.StatusOK, file)
}

func (m *poolManager) handleImport(w http.ResponseWriter, r *http.Request) {
	var file poolFile
	if err := json.NewDecoder(r.Body).Decode(&file); err != nil {
		m.writeErr(w, r, fmt.Errorf("%w: %w", pool.ErrInvalid, err))
		return
	}
	if file.Version != exportVersion {
		m.writeErr(w, r, fmt.Errorf("%w: file version %d, expected %d", pool.ErrInvalid, file.Version, exportVersion))
		return
	}
	if err := m.importPools(r.Context(), file.Pools); err != nil {
		m.writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": len(file.Pools)})
}
