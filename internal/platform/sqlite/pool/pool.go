// Package pool stores the pool definitions of the pool PoC and runs user SQL read-only,
// see knowledge/experiments/0005-pool-poc.md.
package pool

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-sqlite3"
)

const (
	StateRunning = "running"
	StateStopped = "stopped"

	queryTimeout = 10 * time.Second
	maxRows      = 500
	maxBytes     = 5 << 20   // text in one UI result
	maxValue     = 256 << 10 // one string or blob, SQLITE_LIMIT_LENGTH
	maxColumns   = 64        // SQLITE_LIMIT_COLUMN, so that one row has max. 16 MiB
	readOnly     = "sqlite3_pool_ro"

	sqliteRecursive = 33 // SQLITE_RECURSIVE, not exported by go-sqlite3
)

var (
	ErrInvalid  = errors.New("invalid pool")
	ErrNotFound = errors.New("pool not found")
	namePattern = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	reserved    = []string{"ui", "mcp"}
)

type Pool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Query       string `json:"query"`
	State       string `json:"state,omitempty"`
}

func (p Pool) Validate() error {
	if !namePattern.MatchString(p.Name) || slices.Contains(reserved, p.Name) {
		return fmt.Errorf("%w: name %q, permitted: a-z 0-9 -, 1 to 40 characters, not %v", ErrInvalid, p.Name, reserved)
	}
	if strings.TrimSpace(p.Description) == "" {
		return fmt.Errorf("%w: pool %q has no description", ErrInvalid, p.Name)
	}
	if strings.TrimSpace(p.Query) == "" {
		return fmt.Errorf("%w: pool %q has no query", ErrInvalid, p.Name)
	}
	return nil
}

var registerOnce sync.Once

// allowRead is the authorizer of the user SQL connection. SQLite asks it for each statement
// at prepare time, also for each statement of a multi-statement string: only reads pass.
// FTS5 reads PRAGMA data_version internally, so that pragma passes too.
func allowRead(op int, arg1, _, _ string) int {
	switch {
	case op == sqlite3.SQLITE_SELECT, op == sqlite3.SQLITE_READ, op == sqlite3.SQLITE_FUNCTION, op == sqliteRecursive:
		return sqlite3.SQLITE_OK
	case op == sqlite3.SQLITE_PRAGMA && arg1 == "data_version":
		return sqlite3.SQLITE_OK
	}
	return sqlite3.SQLITE_DENY
}

// NewReadOnlyConn opens the DB for user SQL, apart from the pool of the MCP tools,
// see knowledge/experiments/0005-pool-poc.md.
func NewReadOnlyConn(dbPath string) (*sql.DB, error) {
	registerOnce.Do(func() {
		sql.Register(readOnly, &sqlite3.SQLiteDriver{ConnectHook: func(c *sqlite3.SQLiteConn) error {
			c.SetLimit(sqlite3.SQLITE_LIMIT_ATTACHED, 0)
			c.SetLimit(sqlite3.SQLITE_LIMIT_LENGTH, maxValue)
			c.SetLimit(sqlite3.SQLITE_LIMIT_COLUMN, maxColumns)
			c.RegisterAuthorizer(allowRead)
			return nil
		}})
	})
	db, err := sql.Open(readOnly, fmt.Sprintf("file:%s?mode=ro&_query_only=1&_busy_timeout=5000", dbPath))
	if err != nil {
		return nil, fmt.Errorf("opening read-only db connection: %w", err)
	}
	db.SetMaxOpenConns(4)
	return db, nil
}

// userErr marks the errors that the user SQL causes, so that the UI shows them as a bad request.
func userErr(err error) error {
	var sqliteErr sqlite3.Error
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: the query took longer than %s", ErrInvalid, queryTimeout)
	}
	if errors.As(err, &sqliteErr) {
		switch sqliteErr.Code {
		case sqlite3.ErrError, sqlite3.ErrAuth, sqlite3.ErrTooBig, sqlite3.ErrInterrupt, sqlite3.ErrRange, sqlite3.ErrMismatch, sqlite3.ErrReadonly:
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	return err
}

func List(ctx context.Context, db *sql.DB) (pools []Pool, err error) {
	rows, err := db.QueryContext(ctx, `SELECT name, description, query, state FROM pools ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("listing pools: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	for rows.Next() {
		var p Pool
		if err := rows.Scan(&p.Name, &p.Description, &p.Query, &p.State); err != nil {
			return nil, err
		}
		pools = append(pools, p)
	}
	return pools, rows.Err()
}

// Execer is a *sql.DB or a *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func Save(ctx context.Context, db Execer, p Pool) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO pools (name, description, query, state, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET description = excluded.description, query = excluded.query,
			state = excluded.state, updated_at = excluded.updated_at`,
		p.Name, p.Description, p.Query, p.State, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("saving pool %q: %w", p.Name, err)
	}
	return nil
}

func Delete(ctx context.Context, db *sql.DB, name string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM pools WHERE name = ?`, name); err != nil {
		return fmt.Errorf("deleting pool %q: %w", name, err)
	}
	return nil
}

type QueryResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
	// Documents is set when the result is one column of document IDs, so that the UI shows
	// what a pool query selects. The key is the document ID.
	Documents map[int64]DocumentRef `json:"documents,omitempty"`
}

type DocumentRef struct {
	SourceName string `json:"source_name"`
	Title      string `json:"title"`
	URL        string `json:"url,omitempty"`
}

// wrap makes a subquery of the user SQL. It is not the protection: the authorizer is.
// The newline before ")" ends a trailing "--" comment.
func wrap(query string) string {
	query = strings.TrimRight(strings.TrimSpace(query), "; \t\r\n")
	return "SELECT * FROM (\n" + query + "\n)"
}

// Run runs user SQL on the read-only connection and gives max. 500 rows and 5 MiB of text.
func Run(ctx context.Context, ro *sql.DB, query string) (res QueryResult, err error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := ro.QueryContext(ctx, wrap(query))
	if err != nil {
		return res, userErr(err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	if res.Columns, err = rows.Columns(); err != nil {
		return res, err
	}
	res.Rows = [][]any{}
	size := 0
	for rows.Next() {
		if len(res.Rows) == maxRows {
			res.Truncated = true
			break
		}
		values := make([]any, len(res.Columns))
		ptrs := make([]any, len(values))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return res, err
		}
		for i, v := range values {
			switch v := v.(type) {
			case []byte:
				values[i] = string(v)
				size += len(v)
			case string:
				size += len(v)
			}
		}
		if size > maxBytes {
			res.Truncated = true
			break
		}
		res.Rows = append(res.Rows, values)
	}
	if err := rows.Err(); err != nil {
		return res, userErr(err)
	}
	if ids, ok := idColumn(res); ok {
		if res.Documents, err = documentRefs(ctx, ro, ids); err != nil {
			return res, err
		}
	}
	return res, nil
}

func idColumn(res QueryResult) ([]int64, bool) {
	if len(res.Columns) != 1 || len(res.Rows) == 0 {
		return nil, false
	}
	ids := make([]int64, 0, len(res.Rows))
	for _, row := range res.Rows {
		id, ok := row[0].(int64)
		if !ok {
			return nil, false
		}
		ids = append(ids, id)
	}
	return ids, true
}

func documentRefs(ctx context.Context, ro *sql.DB, ids []int64) (refs map[int64]DocumentRef, err error) {
	body, _ := json.Marshal(ids) // []int64 cannot fail
	rows, err := ro.QueryContext(ctx, `
		SELECT id, source_name, title, COALESCE(url, '') FROM pool_documents
		WHERE id IN (SELECT value FROM json_each(?))`, string(body))
	if err != nil {
		return nil, fmt.Errorf("reading the documents of the result: %w", err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	refs = map[int64]DocumentRef{}
	for rows.Next() {
		var id int64
		var ref DocumentRef
		if err := rows.Scan(&id, &ref.SourceName, &ref.Title, &ref.URL); err != nil {
			return nil, err
		}
		refs[id] = ref
	}
	return refs, rows.Err()
}

// Resolve runs a pool query and gives its document IDs, sorted and never nil.
func Resolve(ctx context.Context, ro *sql.DB, query string) (ids []int64, err error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := ro.QueryContext(ctx, wrap(query))
	if err != nil {
		return nil, userErr(err)
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	if len(cols) != 1 {
		return nil, fmt.Errorf("%w: the query must return one column of document IDs, it returns %d columns", ErrInvalid, len(cols))
	}
	ids = []int64{}
	for rows.Next() {
		var v any
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		id, ok := v.(int64)
		if !ok {
			return nil, fmt.Errorf("%w: the query must return integer document IDs, got %T %v", ErrInvalid, v, v)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, userErr(err)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)

	body, _ := json.Marshal(ids) // []int64 cannot fail
	var known int
	if err := ro.QueryRowContext(ctx, `SELECT COUNT(*) FROM documents WHERE id IN (SELECT value FROM json_each(?))`, string(body)).Scan(&known); err != nil {
		return nil, fmt.Errorf("checking the document IDs: %w", err)
	}
	if known != len(ids) {
		return nil, fmt.Errorf("%w: %d of %d IDs are not document IDs", ErrInvalid, len(ids)-known, len(ids))
	}
	return ids, nil
}

type Document struct {
	ID         int64          `json:"id"`
	SourceName string         `json:"source_name"`
	RelPath    string         `json:"rel_path"`
	Title      string         `json:"title"`
	URL        string         `json:"url,omitempty"`
	Metadata   map[string]any `json:"metadata"`
}

// ListDocuments gives all rows of the view pool_documents for the browse table.
func ListDocuments(ctx context.Context, ro *sql.DB) (docs []Document, err error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	rows, err := ro.QueryContext(ctx, `SELECT id, source_name, rel_path, title, COALESCE(url, ''), metadata FROM pool_documents ORDER BY source_name, rel_path`)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, rows.Close()) }()

	docs = []Document{}
	for rows.Next() {
		var d Document
		var metadata sql.NullString
		if err := rows.Scan(&d.ID, &d.SourceName, &d.RelPath, &d.Title, &d.URL, &metadata); err != nil {
			return nil, err
		}
		d.Metadata = map[string]any{}
		if metadata.Valid {
			if err := json.Unmarshal([]byte(metadata.String), &d.Metadata); err != nil {
				return nil, fmt.Errorf("document %d: reading metadata: %w", d.ID, err)
			}
		}
		docs = append(docs, d)
	}
	return docs, rows.Err()
}
