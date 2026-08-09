// Package sqlitetest provides a shared, real-database test fixture for
// packages that need to exercise actual SQL (dynamic table names, vec0
// MATCH semantics, NOT EXISTS filtering) rather than mock database/sql,
// which can't meaningfully verify SQL correctness at all.
package sqlitetest

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"rag/internal/platform/sqlite"
)

// New creates a fresh, isolated SQLite database backed by a temp file (not
// :memory: -- database/sql's connection pooling means pooled connections to
// :memory: are each an independent, empty database unless shared-cache mode
// is used, which is a confusing footgun not worth the marginal speed gain
// here). Schema is initialized the same way sqlite.NewConn does it in
// production. The DB is closed automatically via t.Cleanup.
func New(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sqlite.NewConn(dbPath)
	if err != nil {
		t.Fatalf("sqlitetest.New: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// InsertChunk seeds a minimal documents -> representations -> chunks chain
// and returns the new chunk's ID. Every real chunk row needs a valid
// representation_id, so this exists to keep that boilerplate out of every
// individual test.
func InsertChunk(t *testing.T, db *sql.DB, text string) int64 {
	t.Helper()
	return InsertChunkWithBreadcrumb(t, db, text, "")
}

// InsertChunkWithBreadcrumb is InsertChunk with an explicit breadcrumb, for
// tests that care about it.
func InsertChunkWithBreadcrumb(t *testing.T, db *sql.DB, text, breadcrumb string) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now()

	docRes, err := db.ExecContext(ctx,
		`INSERT INTO documents (created_at, name, sha256) VALUES (?, ?, ?)`,
		now, "test-doc", "deadbeef",
	)
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: inserting document: %v", err)
	}
	docID, err := docRes.LastInsertId()
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: document id: %v", err)
	}

	repRes, err := db.ExecContext(ctx,
		`INSERT INTO representations (created_at, document_id, stage) VALUES (?, ?, ?)`,
		now, docID, "chunk",
	)
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: inserting representation: %v", err)
	}
	repID, err := repRes.LastInsertId()
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: representation id: %v", err)
	}

	chunkRes, err := db.ExecContext(ctx,
		`INSERT INTO chunks (created_at, representation_id, position, text, breadcrumb) VALUES (?, ?, ?, ?, ?)`,
		now, repID, 0, text, breadcrumb,
	)
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: inserting chunk: %v", err)
	}
	chunkID, err := chunkRes.LastInsertId()
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: chunk id: %v", err)
	}

	return chunkID
}
