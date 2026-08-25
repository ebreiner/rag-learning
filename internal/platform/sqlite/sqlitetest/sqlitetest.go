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
	db, err := sqlite.NewConn(dbPath, false)
	if err != nil {
		t.Fatalf("sqlitetest.New: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("sqlitetest.New: closing db: %v", err)
		}
	})

	return db
}

// InsertChunk seeds a minimal documents -> chunks chain and returns the new
// chunk's ID. Every real chunk row needs a valid document_id, so this exists
// to keep that boilerplate out of every individual test.
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

	_, err := db.ExecContext(ctx,
		`INSERT INTO collections (name, weight) VALUES (?, ?) ON CONFLICT(name) DO NOTHING`,
		"test-collection", 1.0,
	)
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: inserting collection: %v", err)
	}

	docRes, err := db.ExecContext(ctx,
		`INSERT INTO documents (created_at, name, sha256, collection_name) VALUES (?, ?, ?, ?)`,
		now, "test-doc", "deadbeef", "test-collection",
	)
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: inserting document: %v", err)
	}
	docID, err := docRes.LastInsertId()
	if err != nil {
		t.Fatalf("sqlitetest.InsertChunk: document id: %v", err)
	}

	chunkRes, err := db.ExecContext(ctx,
		`INSERT INTO chunks (created_at, document_id, position, text, breadcrumb) VALUES (?, ?, ?, ?, ?)`,
		now, docID, 0, text, breadcrumb,
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
