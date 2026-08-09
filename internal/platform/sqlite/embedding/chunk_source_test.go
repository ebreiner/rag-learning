package embedding

import (
	"context"
	"testing"

	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/sqlitetest"
)

func TestChunksSourceNextChunks(t *testing.T) {
	t.Run("returns a chunk that has no row in the target embeddings table", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		tableName, err := sqlite.SetupVecTable(db, ctx, 4, "modelA")
		if err != nil {
			t.Fatalf("SetupTable: %v", err)
		}

		id := sqlitetest.InsertChunk(t, db, "needs embedding")

		src, err := NewChunkSource(db, ctx, tableName)
		if err != nil {
			t.Fatalf("NewChunkSource: %v", err)
		}

		got, err := src.NextChunks(10)
		if err != nil {
			t.Fatalf("NextChunks() error = %v", err)
		}
		if len(got) != 1 || got[0].ChunkID != id {
			t.Fatalf("got %+v, want exactly chunk %d", got, id)
		}
	})

	t.Run("excludes a chunk that already has a row in the target embeddings table -- this is the core fix for the multi-model design", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		tableName, err := sqlite.SetupVecTable(db, ctx, 4, "modelA")
		if err != nil {
			t.Fatalf("SetupTable: %v", err)
		}

		id := sqlitetest.InsertChunk(t, db, "already embedded")
		packed, err := sqlite.PackVector([]float64{0.1, 0.2, 0.3, 0.4})
		if err != nil {
			t.Fatalf("PackVector: %v", err)
		}
		if _, err := db.Exec("INSERT INTO "+tableName+"(chunk_id, embedding) VALUES (?, ?)", id, packed); err != nil {
			t.Fatalf("seeding embedding row: %v", err)
		}

		src, err := NewChunkSource(db, ctx, tableName)
		if err != nil {
			t.Fatalf("NewChunkSource: %v", err)
		}

		got, err := src.NextChunks(10)
		if err != nil {
			t.Fatalf("NextChunks() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d chunks, want 0 -- chunk %d already has an embedding in %s", len(got), id, tableName)
		}
	})

	t.Run("the same chunk is needed under a second model even though it's done under the first -- the actual backfill property", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		tableA, err := sqlite.SetupVecTable(db, ctx, 4, "modelA")
		if err != nil {
			t.Fatalf("SetupTable(modelA): %v", err)
		}
		tableB, err := sqlite.SetupVecTable(db, ctx, 4, "modelB")
		if err != nil {
			t.Fatalf("SetupTable(modelB): %v", err)
		}

		id := sqlitetest.InsertChunk(t, db, "embedded under A only")
		packed, err := sqlite.PackVector([]float64{0.1, 0.2, 0.3, 0.4})
		if err != nil {
			t.Fatalf("PackVector: %v", err)
		}
		if _, err := db.Exec("INSERT INTO "+tableA+"(chunk_id, embedding) VALUES (?, ?)", id, packed); err != nil {
			t.Fatalf("seeding embedding row under modelA: %v", err)
		}

		srcA, err := NewChunkSource(db, ctx, tableA)
		if err != nil {
			t.Fatalf("NewChunkSource(A): %v", err)
		}
		gotA, err := srcA.NextChunks(10)
		if err != nil {
			t.Fatalf("NextChunks(A) error = %v", err)
		}
		if len(gotA) != 0 {
			t.Fatalf("modelA source should see nothing left to embed, got %d", len(gotA))
		}

		srcB, err := NewChunkSource(db, ctx, tableB)
		if err != nil {
			t.Fatalf("NewChunkSource(B): %v", err)
		}
		gotB, err := srcB.NextChunks(10)
		if err != nil {
			t.Fatalf("NextChunks(B) error = %v", err)
		}
		if len(gotB) != 1 || gotB[0].ChunkID != id {
			t.Fatalf("modelB source should still see chunk %d needing embedding, got %+v", id, gotB)
		}
	})

	t.Run("pagination advances past chunks already returned in an earlier call", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		tableName, err := sqlite.SetupVecTable(db, ctx, 4, "modelA")
		if err != nil {
			t.Fatalf("SetupTable: %v", err)
		}

		var ids []int64
		for range 3 {
			ids = append(ids, sqlitetest.InsertChunk(t, db, "chunk"))
		}

		src, err := NewChunkSource(db, ctx, tableName)
		if err != nil {
			t.Fatalf("NewChunkSource: %v", err)
		}

		first, err := src.NextChunks(2)
		if err != nil {
			t.Fatalf("first NextChunks() error = %v", err)
		}
		if len(first) != 2 {
			t.Fatalf("first call: got %d chunks, want 2", len(first))
		}

		second, err := src.NextChunks(2)
		if err != nil {
			t.Fatalf("second NextChunks() error = %v", err)
		}
		if len(second) != 1 {
			t.Fatalf("second call: got %d chunks, want 1 (the remaining one)", len(second))
		}
		if second[0].ChunkID != ids[2] {
			t.Fatalf("second call returned chunk %d, want the last-inserted chunk %d", second[0].ChunkID, ids[2])
		}
	})

	t.Run("no chunks needing embedding returns an empty result without error", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		tableName, err := sqlite.SetupVecTable(db, ctx, 4, "modelA")
		if err != nil {
			t.Fatalf("SetupTable: %v", err)
		}

		src, err := NewChunkSource(db, ctx, tableName)
		if err != nil {
			t.Fatalf("NewChunkSource: %v", err)
		}

		got, err := src.NextChunks(10)
		if err != nil {
			t.Fatalf("NextChunks() error = %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("got %d chunks, want 0", len(got))
		}
	})
}
