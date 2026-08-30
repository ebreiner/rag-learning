package retrieval

import (
	"context"
	"testing"

	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"
)

func TestWeighChunks(t *testing.T) {
	t.Run("resolves weight per chunk id via the documents -> collections join", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		manualID := insertChunkInCollection(t, db, "manual chunk", "manual", 0.8)
		changelogID := insertChunkInCollection(t, db, "changelog chunk", "changelog", 0.3)

		weigher, err := NewCollectionWeigher(ctx, db, testLogger)
		if err != nil {
			t.Fatalf("NewCollectionWeigher() error = %v", err)
		}

		got, err := weigher.WeighChunks(ctx, []step.ScoredChunkID{{ID: manualID}, {ID: changelogID}})
		if err != nil {
			t.Fatalf("WeighChunks() error = %v", err)
		}

		if got[manualID] != 0.8 {
			t.Errorf("weight[manual] = %v, want 0.8", got[manualID])
		}
		if got[changelogID] != 0.3 {
			t.Errorf("weight[changelog] = %v, want 0.3", got[changelogID])
		}
	})

	t.Run("a chunk id with no matching row is simply absent from the result, not an error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		realID := insertChunkInCollection(t, db, "real chunk", "manual", 1.0)

		weigher, err := NewCollectionWeigher(ctx, db, testLogger)
		if err != nil {
			t.Fatalf("NewCollectionWeigher() error = %v", err)
		}

		got, err := weigher.WeighChunks(ctx, []step.ScoredChunkID{{ID: realID}, {ID: 999999}})
		if err != nil {
			t.Fatalf("WeighChunks() error = %v, want nil", err)
		}
		if _, ok := got[999999]; ok {
			t.Errorf("weight map contains an entry for a nonexistent chunk id, want it absent")
		}
		if got[realID] != 1.0 {
			t.Errorf("weight[real] = %v, want 1.0", got[realID])
		}
	})

	t.Run("empty input produces an empty result, not an error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		insertChunkInCollection(t, db, "seed chunk", "manual", 1.0)

		weigher, err := NewCollectionWeigher(ctx, db, testLogger)
		if err != nil {
			t.Fatalf("NewCollectionWeigher() error = %v", err)
		}

		got, err := weigher.WeighChunks(ctx, nil)
		if err != nil {
			t.Fatalf("WeighChunks() error = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d entries, want 0", len(got))
		}
	})
}
