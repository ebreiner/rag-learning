package embedding

import (
	"context"
	"database/sql"
	"testing"

	"rag/internal/embedding/step"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/sqlitetest"
)

func TestSaveEmbeddings(t *testing.T) {
	t.Run("saves every embedding into the correct model-keyed table", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id1 := sqlitetest.InsertChunk(t, db, "first chunk")
		id2 := sqlitetest.InsertChunk(t, db, "second chunk")

		sink, err := NewEmbeddingsResultSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewEmbeddingsResultSink: %v", err)
		}

		toSave := step.EmbeddingsToSave{
			Model: "bge-m3",
			Dim:   3,
			Embeddings: []step.Embedding{
				{ChunkID: id1, Vector: []float64{0.1, 0.2, 0.3}},
				{ChunkID: id2, Vector: []float64{0.4, 0.5, 0.6}},
			},
		}
		if err := sink.SaveEmbeddings(toSave, ctx); err != nil {
			t.Fatalf("SaveEmbeddings() error = %v", err)
		}

		var count int
		if err := db.QueryRow("SELECT count(*) FROM embeddings_bge_m3_3").Scan(&count); err != nil {
			t.Fatalf("querying embeddings_bge_m3_3: %v", err)
		}
		if count != 2 {
			t.Errorf("got %d rows in embeddings_bge_m3_3, want 2", count)
		}
	})

	t.Run("creates the target table if it doesn't exist yet", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id := sqlitetest.InsertChunk(t, db, "a chunk")

		sink, err := NewEmbeddingsResultSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewEmbeddingsResultSink: %v", err)
		}

		toSave := step.EmbeddingsToSave{
			Model:      "brand-new-model",
			Dim:        2,
			Embeddings: []step.Embedding{{ChunkID: id, Vector: []float64{0.1, 0.2}}},
		}
		if err := sink.SaveEmbeddings(toSave, ctx); err != nil {
			t.Fatalf("SaveEmbeddings() error = %v", err)
		}

		var count int
		if err := db.QueryRow("SELECT count(*) FROM embeddings_brand_new_model_2").Scan(&count); err != nil {
			t.Fatalf("expected embeddings_brand_new_model_2 to exist: %v", err)
		}
		if count != 1 {
			t.Errorf("got %d rows, want 1", count)
		}
	})

	t.Run("a failure partway through rolls back the whole batch, not just the failing row", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id1 := sqlitetest.InsertChunk(t, db, "good chunk")

		sink, err := NewEmbeddingsResultSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewEmbeddingsResultSink: %v", err)
		}

		// second embedding has a vector length that won't match the
		// table's declared dimension (3), which vec0 should reject.
		toSave := step.EmbeddingsToSave{
			Model: "rollback-test",
			Dim:   3,
			Embeddings: []step.Embedding{
				{ChunkID: id1, Vector: []float64{0.1, 0.2, 0.3}},
				{ChunkID: 999999, Vector: []float64{0.1, 0.2}}, // wrong length, should be rejected
			},
		}
		if err := sink.SaveEmbeddings(toSave, ctx); err == nil {
			t.Fatalf("expected SaveEmbeddings() to fail on the malformed second embedding")
		}

		var count int
		if err := db.QueryRow("SELECT count(*) FROM embeddings_rollback_test_3").Scan(&count); err != nil {
			t.Fatalf("querying embeddings_rollback_test_3: %v", err)
		}
		if count != 0 {
			t.Errorf("got %d rows after a failed save, want 0 -- the first (valid) row should have been rolled back too", count)
		}
	})

	t.Run("packed vectors round-trip correctly through a real MATCH query", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		id := sqlitetest.InsertChunk(t, db, "a chunk")

		sink, err := NewEmbeddingsResultSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewEmbeddingsResultSink: %v", err)
		}

		want := []float64{0.1, 0.2, 0.3, 0.4}
		toSave := step.EmbeddingsToSave{
			Model:      "roundtrip-test",
			Dim:        4,
			Embeddings: []step.Embedding{{ChunkID: id, Vector: want}},
		}
		if err := sink.SaveEmbeddings(toSave, ctx); err != nil {
			t.Fatalf("SaveEmbeddings() error = %v", err)
		}

		packed, err := sqlite.PackVector(want)
		if err != nil {
			t.Fatalf("packing query vector: %v", err)
		}
		var distance sql.NullFloat64
		row := db.QueryRow(
			"SELECT distance FROM embeddings_roundtrip_test_4 WHERE embedding MATCH ? ORDER BY distance LIMIT 1",
			packed,
		)
		if err := row.Scan(&distance); err != nil {
			t.Fatalf("MATCH query: %v", err)
		}
		if !distance.Valid || distance.Float64 > 0.0001 {
			t.Errorf("distance to itself = %v, want ~0", distance)
		}
	})
}
