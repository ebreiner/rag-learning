package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"
)

// insertChunkInCollection seeds a document+chunk under an explicit collection
// name/weight (upserting the collection row too), unlike sqlitetest.InsertChunk
// which always uses a fixed "test-collection". Needed here because the
// hydrator's whole job is joining through collection_name -> collections.weight.
func insertChunkInCollection(t *testing.T, db *sql.DB, text, collectionName string, weight float64) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO collections (name, weight) VALUES (?, ?) ON CONFLICT(name) DO UPDATE SET weight = excluded.weight`,
		collectionName, weight,
	); err != nil {
		t.Fatalf("insertChunkInCollection: upserting collection: %v", err)
	}

	docRes, err := db.ExecContext(ctx,
		`INSERT INTO documents (created_at, name, sha256, collection_name) VALUES (?, ?, ?, ?)`,
		now, "test-doc", fmt.Sprintf("sha-%s-%d", collectionName, time.Now().UnixNano()), collectionName,
	)
	if err != nil {
		t.Fatalf("insertChunkInCollection: inserting document: %v", err)
	}
	docID, err := docRes.LastInsertId()
	if err != nil {
		t.Fatalf("insertChunkInCollection: document id: %v", err)
	}

	chunkRes, err := db.ExecContext(ctx,
		`INSERT INTO chunks (created_at, document_id, position, type, text, breadcrumb) VALUES (?, ?, ?, ?, ?, ?)`,
		now, docID, 0, "content", text, "",
	)
	if err != nil {
		t.Fatalf("insertChunkInCollection: inserting chunk: %v", err)
	}
	chunkID, err := chunkRes.LastInsertId()
	if err != nil {
		t.Fatalf("insertChunkInCollection: chunk id: %v", err)
	}

	return chunkID
}

func TestNewChunkHydrator(t *testing.T) {
	t.Run("errors when the collections table is empty", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()

		_, err := NewChunkHydrator(ctx, db, testLogger)
		if err == nil {
			t.Fatalf("NewChunkHydrator() error = nil, want an error for an empty collections table")
		}
	})

	t.Run("succeeds once at least one collection exists", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		insertChunkInCollection(t, db, "seed chunk", "manual", 1.0)

		if _, err := NewChunkHydrator(ctx, db, testLogger); err != nil {
			t.Fatalf("NewChunkHydrator() error = %v", err)
		}
	})
}

func TestHydrateChunks(t *testing.T) {
	t.Run("populates collection name and weight via the documents join", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		manualID := insertChunkInCollection(t, db, "manual chunk", "manual", 0.8)
		changelogID := insertChunkInCollection(t, db, "changelog chunk", "changelog", 0.3)

		hydrator, err := NewChunkHydrator(ctx, db, testLogger)
		if err != nil {
			t.Fatalf("NewChunkHydrator() error = %v", err)
		}

		chunks, err := hydrator.HydrateChunks(ctx, []step.ScoredChunkID{{ID: manualID}, {ID: changelogID}})
		if err != nil {
			t.Fatalf("HydrateChunks() error = %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}

		byID := make(map[int64]step.RetrievedChunk, len(chunks))
		for _, c := range chunks {
			byID[c.ID] = c
		}

		if got := byID[manualID]; got.CollectionName != "manual" || got.CollectionWeight != 0.8 {
			t.Errorf("manual chunk collection = (%q, %v), want (%q, %v)", got.CollectionName, got.CollectionWeight, "manual", 0.8)
		}
		if got := byID[changelogID]; got.CollectionName != "changelog" || got.CollectionWeight != 0.3 {
			t.Errorf("changelog chunk collection = (%q, %v), want (%q, %v)", got.CollectionName, got.CollectionWeight, "changelog", 0.3)
		}
	})

	t.Run("errors when a chunk's collection was created after the hydrator snapshot was taken", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		insertChunkInCollection(t, db, "early chunk", "early-collection", 1.0)

		hydrator, err := NewChunkHydrator(ctx, db, testLogger)
		if err != nil {
			t.Fatalf("NewChunkHydrator() error = %v", err)
		}

		// Simulates a serve-time extract happening after the server (and its
		// hydrator's collection-weight snapshot) already started.
		lateID := insertChunkInCollection(t, db, "late chunk", "late-collection", 0.5)

		chunks, err := hydrator.HydrateChunks(ctx, []step.ScoredChunkID{{ID: lateID}})
		if err == nil {
			t.Fatalf("HydrateChunks() error = nil, want an error for a collection unknown to the snapshot, got chunks %+v", chunks)
		}
	})

	t.Run("a chunk id with no matching row is skipped, not an error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		insertChunkInCollection(t, db, "real chunk", "manual", 1.0)

		hydrator, err := NewChunkHydrator(ctx, db, testLogger)
		if err != nil {
			t.Fatalf("NewChunkHydrator() error = %v", err)
		}

		chunks, err := hydrator.HydrateChunks(ctx, []step.ScoredChunkID{{ID: 999999}})
		if err != nil {
			t.Fatalf("HydrateChunks() error = %v, want nil", err)
		}
		if len(chunks) != 0 {
			t.Errorf("got %d chunks, want 0", len(chunks))
		}
	})

	t.Run("output order follows the order of the requested ids, not db insertion order, and Rank/Score pass through unchanged", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		idA := insertChunkInCollection(t, db, "chunk A", "manual", 1.0)
		idB := insertChunkInCollection(t, db, "chunk B", "manual", 1.0)

		hydrator, err := NewChunkHydrator(ctx, db, testLogger)
		if err != nil {
			t.Fatalf("NewChunkHydrator() error = %v", err)
		}

		// Rank/Score are set here because HydrateChunks no longer derives
		// Rank from position -- it's the caller's job (rrfMerge/runFTS/
		// runANN/collectionRerank) to have already assigned the real final
		// Rank before hydration ever runs; HydrateChunks just carries it
		// through onto the RetrievedChunk unchanged.
		chunks, err := hydrator.HydrateChunks(ctx, []step.ScoredChunkID{
			{ID: idB, Rank: 1, Score: 0.9},
			{ID: idA, Rank: 2, Score: 0.5},
		})
		if err != nil {
			t.Fatalf("HydrateChunks() error = %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}
		if chunks[0].ID != idB || chunks[0].Rank != 1 || chunks[0].Score != 0.9 {
			t.Errorf("chunks[0] = (ID: %d, Rank: %d, Score: %v), want (ID: %d, Rank: 1, Score: 0.9)", chunks[0].ID, chunks[0].Rank, chunks[0].Score, idB)
		}
		if chunks[1].ID != idA || chunks[1].Rank != 2 || chunks[1].Score != 0.5 {
			t.Errorf("chunks[1] = (ID: %d, Rank: %d, Score: %v), want (ID: %d, Rank: 2, Score: 0.5)", chunks[1].ID, chunks[1].Rank, chunks[1].Score, idA)
		}
	})
}
