package retrieval

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"
)

var testLogger = slog.New(slog.DiscardHandler)

func seedEmbedding(t *testing.T, db *sql.DB, tableName string, chunkID int64, vec []float64) {
	t.Helper()
	packed, err := sqlite.PackVector(vec)
	if err != nil {
		t.Fatalf("PackVector: %v", err)
	}
	if _, err := db.Exec("INSERT INTO "+tableName+"(chunk_id, embedding) VALUES (?, ?)", chunkID, packed); err != nil {
		t.Fatalf("seeding embedding row: %v", err)
	}
}

func TestTopKByANN(t *testing.T) {
	t.Run("resolves the table from the query's model and dim, and orders by distance", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		tableName, err := sqlite.SetupVecTable(db, ctx, 3, "bge-m3")
		if err != nil {
			t.Fatalf("SetupTable: %v", err)
		}

		idClose := sqlitetest.InsertChunk(t, db, "close chunk")
		idFar := sqlitetest.InsertChunk(t, db, "far chunk")
		seedEmbedding(t, db, tableName, idClose, []float64{1, 0, 0})
		seedEmbedding(t, db, tableName, idFar, []float64{0, 0, 1})

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByANN(step.Query{Vector: []float64{1, 0, 0}, Dim: 3, Model: "bge-m3"}, 10, ctx)
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d results, want 2", len(got))
		}
		if got[0] != idClose {
			t.Errorf("closest result = %d, want %d (the identical vector)", got[0], idClose)
		}
	})

	t.Run("an empty table returns an empty result, not an error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		if _, err := sqlite.SetupVecTable(db, ctx, 3, "empty-model"); err != nil {
			t.Fatalf("SetupTable: %v", err)
		}

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByANN(step.Query{Vector: []float64{1, 0, 0}, Dim: 3, Model: "empty-model"}, 10, ctx)
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d results, want 0", len(got))
		}
	})

	t.Run("cross-model safety: querying a model that has real embeddings never returns another model's chunks", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		tableA, err := sqlite.SetupVecTable(db, ctx, 3, "modelA")
		if err != nil {
			t.Fatalf("SetupTable(modelA): %v", err)
		}
		idA := sqlitetest.InsertChunk(t, db, "chunk under modelA")
		seedEmbedding(t, db, tableA, idA, []float64{1, 0, 0})

		tableB, err := sqlite.SetupVecTable(db, ctx, 3, "modelB")
		if err != nil {
			t.Fatalf("SetupTable(modelB): %v", err)
		}
		idB := sqlitetest.InsertChunk(t, db, "chunk under modelB")
		seedEmbedding(t, db, tableB, idB, []float64{0, 1, 0})

		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByANN(step.Query{Vector: []float64{1, 0, 0}, Dim: 3, Model: "modelB"}, 10, ctx)
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		for _, id := range got {
			if id == idA {
				t.Fatalf("querying modelB returned modelA's chunk %d -- vector spaces leaked across models", idA)
			}
		}
	})

	// TopKByANN resolves the table via sqlite.LookupVecTable, which errors
	// instead of creating one. A query for a model that was never embedded
	// -- including a plain typo in --model -- fails clearly rather than
	// silently returning zero results against a freshly created, permanently
	// empty table.
	t.Run("querying a never-embedded model errors instead of silently creating a stray table", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		retriever, err := NewSQLiteRetriever(db, testLogger)
		if err != nil {
			t.Fatalf("NewSQLiteRetriever: %v", err)
		}

		got, err := retriever.TopKByANN(step.Query{Vector: []float64{1, 0, 0}, Dim: 3, Model: "typo-model"}, 10, ctx)
		if err == nil {
			t.Fatalf("TopKByANN() error = nil, want an error for an unknown model")
		}
		if len(got) != 0 {
			t.Fatalf("got %d results, want 0", len(got))
		}

		var count int
		if err := db.QueryRow("SELECT count(*) FROM embeddings_typo_model_3").Scan(&count); err == nil {
			t.Fatalf("expected no stray embeddings_typo_model_3 table to have been created, but querying it succeeded: %v", err)
		}
	})
}
