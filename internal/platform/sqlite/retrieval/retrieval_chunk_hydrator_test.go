package retrieval

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"
)

// insertChunkInSource seeds a document+chunk under an explicit source
// name/weight (upserting the source row too), unlike sqlitetest.InsertChunk
// which always uses sqlitetest.TestSource. Needed here because the
// hydrator's whole job is joining through source_name -> sources.weight.
// docSeq keeps UNIQUE(source_name, rel_path) satisfied, the tests in this package do not run in parallel.
var docSeq int

func insertChunkInSource(t *testing.T, db *sql.DB, text, sourceName string, weight float64) int64 {
	t.Helper()
	ctx := context.Background()
	now := time.Now()

	if _, err := db.ExecContext(ctx,
		`INSERT INTO sources (name, weight) VALUES (?, ?) ON CONFLICT(name) DO UPDATE SET weight = excluded.weight`,
		sourceName, weight,
	); err != nil {
		t.Fatalf("insertChunkInSource: upserting source: %v", err)
	}

	docSeq++
	docRes, err := db.ExecContext(ctx,
		`INSERT INTO documents (created_at, name, rel_path, sha256, source_name) VALUES (?, ?, ?, ?, ?)`,
		now, "test-doc", fmt.Sprintf("test-doc-%d", docSeq), fmt.Sprintf("sha-%s-%d", sourceName, docSeq), sourceName,
	)
	if err != nil {
		t.Fatalf("insertChunkInSource: inserting document: %v", err)
	}
	docID, err := docRes.LastInsertId()
	if err != nil {
		t.Fatalf("insertChunkInSource: document id: %v", err)
	}

	chunkRes, err := db.ExecContext(ctx,
		`INSERT INTO chunks (created_at, document_id, position, type, text, breadcrumb) VALUES (?, ?, ?, ?, ?, ?)`,
		now, docID, 0, "content", text, "",
	)
	if err != nil {
		t.Fatalf("insertChunkInSource: inserting chunk: %v", err)
	}
	chunkID, err := chunkRes.LastInsertId()
	if err != nil {
		t.Fatalf("insertChunkInSource: chunk id: %v", err)
	}

	return chunkID
}

func TestHydrateChunks(t *testing.T) {
	t.Run("populates source name and weight via the documents join", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		manualID := insertChunkInSource(t, db, "manual chunk", "manual", 0.8)
		changelogID := insertChunkInSource(t, db, "changelog chunk", "changelog", 0.3)

		hydrator := NewChunkHydrator(db, testLogger)

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

		if got := byID[manualID]; got.SourceName != "manual" || got.SourceWeight != 0.8 {
			t.Errorf("manual chunk source = (%q, %v), want (%q, %v)", got.SourceName, got.SourceWeight, "manual", 0.8)
		}
		if got := byID[changelogID]; got.SourceName != "changelog" || got.SourceWeight != 0.3 {
			t.Errorf("changelog chunk source = (%q, %v), want (%q, %v)", got.SourceName, got.SourceWeight, "changelog", 0.3)
		}
	})

	t.Run("hydrates a chunk whose source was created after the hydrator was constructed", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		insertChunkInSource(t, db, "early chunk", "early-source", 1.0)

		hydrator := NewChunkHydrator(db, testLogger)

		// Simulates an extract run adding a new source while `serve mcp`
		// is already up. The old startup snapshot made every query touching
		// the new source fail until restart; the weight now comes from
		// the hydration query's join, so there is no snapshot to go stale.
		lateID := insertChunkInSource(t, db, "late chunk", "late-source", 0.5)

		chunks, err := hydrator.HydrateChunks(ctx, []step.ScoredChunkID{{ID: lateID}})
		if err != nil {
			t.Fatalf("HydrateChunks() error = %v, want nil for a source created after construction", err)
		}
		if len(chunks) != 1 {
			t.Fatalf("got %d chunks, want 1", len(chunks))
		}
		if got := chunks[0]; got.SourceName != "late-source" || got.SourceWeight != 0.5 {
			t.Errorf("late chunk source = (%q, %v), want (%q, %v)", got.SourceName, got.SourceWeight, "late-source", 0.5)
		}
	})

	t.Run("a chunk id with no matching row is skipped, not an error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		insertChunkInSource(t, db, "real chunk", "manual", 1.0)

		hydrator := NewChunkHydrator(db, testLogger)

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
		idA := insertChunkInSource(t, db, "chunk A", "manual", 1.0)
		idB := insertChunkInSource(t, db, "chunk B", "manual", 1.0)

		hydrator := NewChunkHydrator(db, testLogger)

		// Rank/Score are set here because HydrateChunks no longer derives
		// Rank from position -- it's the caller's job (rrfMerge/runFTS/
		// runANN/sourceRerank) to have already assigned the real final
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

func TestHydrateChunksMetadata(t *testing.T) {
	t.Run("metadata_json becomes Metadata, NULL gives an empty map", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		withID := insertChunkInSource(t, db, "with metadata", "manual", 1.0)
		withoutID := insertChunkInSource(t, db, "without metadata", "manual", 1.0)
		if _, err := db.Exec(`UPDATE documents SET metadata_json = ? WHERE id = (SELECT document_id FROM chunks WHERE id = ?)`,
			`{"classification.personal_data": true, "title": "A"}`, withID); err != nil {
			t.Fatalf("setting metadata_json: %v", err)
		}

		hydrator := NewChunkHydrator(db, testLogger)
		chunks, err := hydrator.HydrateChunks(ctx, []step.ScoredChunkID{{ID: withID}, {ID: withoutID}})
		if err != nil {
			t.Fatalf("HydrateChunks() error = %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}

		want := map[string]any{"classification.personal_data": true, "title": "A"}
		if diff := cmp.Diff(want, chunks[0].Metadata); diff != "" {
			t.Errorf("metadata (-want +got):\n%s", diff)
		}
		if chunks[1].Metadata == nil || len(chunks[1].Metadata) != 0 {
			t.Errorf("metadata without metadata_json = %v, want an empty map", chunks[1].Metadata)
		}
	})
}
