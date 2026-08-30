package retrieval

import (
	"context"
	"testing"

	"rag/internal/platform/sqlite/sqlitetest"
)

func TestTopKByFTS(t *testing.T) {
	t.Run("finds a chunk containing the query term", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id := sqlitetest.InsertChunk(t, db, "single sign on via auth0 configuration")
		sqlitetest.InsertChunk(t, db, "completely unrelated content about ldap servers")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByFTS(ctx, "auth0", 10)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v", err)
		}
		if len(got) != 1 || got[0].ID != id {
			t.Fatalf("got %v, want exactly [%d]", got, id)
		}
	})

	t.Run("orders results by relevance", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		idStrong := sqlitetest.InsertChunk(t, db, "auth0 auth0 auth0 configuration for single sign on")
		idWeak := sqlitetest.InsertChunk(t, db, "a passing mention of auth0 among other unrelated topics entirely")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByFTS(ctx, "auth0", 10)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d results, want 2", len(got))
		}
		if got[0].ID != idStrong {
			t.Errorf("top result = %d, want %d (the chunk repeating the term should rank first)", got[0].ID, idStrong)
		}
		if got[1].ID != idWeak {
			t.Errorf("second result = %d, want %d", got[1].ID, idWeak)
		}
	})

	t.Run("no matching chunks returns an empty result, not an error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		sqlitetest.InsertChunk(t, db, "something completely unrelated")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByFTS(ctx, "nonexistent-term-xyz", 10)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d results, want 0", len(got))
		}
	})

	t.Run("a multi-word query matches a chunk containing only one of the words -- terms are OR-joined", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id := sqlitetest.InsertChunk(t, db, "configuring ldap server settings")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByFTS(ctx, "auth0 ldap", 10)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v", err)
		}
		if len(got) != 1 || got[0].ID != id {
			t.Fatalf("got %v, want [%d] -- chunk contains 'ldap' even though it lacks 'auth0'", got, id)
		}
	})

	t.Run("short words (<=2 runes) are dropped from the OR list but the query still matches via the raw fallback term", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id := sqlitetest.InsertChunk(t, db, "an ldap configuration guide")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		// "an" is <=2 runes and gets filtered from the term list, but the
		// whole raw query is always appended as a final term (fts_retrieval.go),
		// so "an ldap configuration guide" as a literal phrase should still match.
		got, err := retriever.TopKByFTS(ctx, "an ldap configuration guide", 10)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v", err)
		}
		if len(got) != 1 || got[0].ID != id {
			t.Fatalf("got %v, want [%d]", got, id)
		}
	})

	t.Run("a query made entirely of short words still matches via the raw fallback term", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id := sqlitetest.InsertChunk(t, db, "it is on at go")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByFTS(ctx, "it is on", 10)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v", err)
		}
		if len(got) != 1 || got[0].ID != id {
			t.Fatalf("got %v, want [%d] -- every word is <=2 runes, so only the raw-query fallback term can match", got, id)
		}
	})

	t.Run("a literal double quote in the query is escaped, not a syntax error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		sqlitetest.InsertChunk(t, db, "some content mentioning auth0 settings")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		_, err = retriever.TopKByFTS(ctx, `auth0 "quoted" settings`, 10)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v, want no error -- the quote should be escaped, not break the MATCH syntax", err)
		}
	})

	t.Run("k limits the number of results", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		for range 5 {
			sqlitetest.InsertChunk(t, db, "auth0 configuration document")
		}

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByFTS(ctx, "auth0", 2)
		if err != nil {
			t.Fatalf("TopKByFTS() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d results, want 2 (k should limit the result set)", len(got))
		}
	})

	// documents current behavior for an edge case rather than asserting it's
	// the only correct behavior: an empty query string still reaches the FTS
	// MATCH expression as an empty quoted term. Pinning this down so it's a
	// deliberate decision if it ever changes, not an unnoticed regression.
	t.Run("empty query string does not panic or error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		sqlitetest.InsertChunk(t, db, "some content")

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByFTS(ctx, "", 10)
		if err != nil {
			t.Fatalf("TopKByFTS(\"\") error = %v (if this now errors, that's a behavior change -- update this test deliberately)", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d results for an empty query, want 0", len(got))
		}
	})
}
