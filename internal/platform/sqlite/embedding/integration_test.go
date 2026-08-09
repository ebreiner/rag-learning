package embedding

import (
	"context"
	"database/sql"
	"testing"

	"rag/internal/embedding/step"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/sqlitetest"
)

// fakeEmbedClient produces deterministic, cheap vectors without any real
// network call -- these integration tests are about the wiring between
// ChunksSource, ResultSink, and Embed() against a real database, not about
// the HTTP client (which is already covered separately with httptest).
type fakeEmbedClient struct {
	model string
	dim   int64
	calls int
}

func (f *fakeEmbedClient) EmbedChunks(chunks []step.ChunkToEmbed) (step.EmbeddingsToSave, error) {
	f.calls++
	embeddings := make([]step.Embedding, len(chunks))
	for i, c := range chunks {
		vec := make([]float64, f.dim)
		for j := range vec {
			vec[j] = 0.1
		}
		embeddings[i] = step.Embedding{ChunkID: c.ChunkID, Vector: vec}
	}
	return step.EmbeddingsToSave{Model: f.model, Dim: f.dim, Embeddings: embeddings}, nil
}

func embedAll(t *testing.T, db *sql.DB, model string, dim int64) *fakeEmbedClient {
	t.Helper()
	ctx := context.Background()
	tableName, err := sqlite.SetupVecTable(db, ctx, dim, model)
	if err != nil {
		t.Fatalf("SetupTable(%s): %v", model, err)
	}
	source, err := NewChunkSource(db, ctx, tableName)
	if err != nil {
		t.Fatalf("NewChunkSource(%s): %v", model, err)
	}
	sink, err := NewEmbeddingsResultSink(db, ctx)
	if err != nil {
		t.Fatalf("NewEmbeddingsResultSink: %v", err)
	}
	client := &fakeEmbedClient{model: model, dim: dim}

	if err := step.Embed(&sink, &source, client); err != nil {
		t.Fatalf("Embed(%s): %v", model, err)
	}
	return client
}

func TestEmbedIntegration(t *testing.T) {
	t.Run("re-running embed for the same model does not re-embed anything", func(t *testing.T) {
		db := sqlitetest.New(t)
		for range 5 {
			sqlitetest.InsertChunk(t, db, "a real chunk of text")
		}

		first := embedAll(t, db, "bge-m3", 4)
		if first.calls != 1 {
			t.Fatalf("first run: EmbedChunks called %d times, want 1", first.calls)
		}

		second := embedAll(t, db, "bge-m3", 4)
		if second.calls != 0 {
			t.Fatalf("second run: EmbedChunks called %d times, want 0 -- nothing should have been left to embed", second.calls)
		}

		var count int
		if err := db.QueryRow("SELECT count(*) FROM embeddings_bge_m3_4").Scan(&count); err != nil {
			t.Fatalf("querying embeddings_bge_m3_4: %v", err)
		}
		if count != 5 {
			t.Fatalf("got %d rows, want 5 (no duplicates from the second run)", count)
		}
	})

	t.Run("adding a second model embeds every existing chunk under it too, automatically -- the actual design goal of this whole rework", func(t *testing.T) {
		db := sqlitetest.New(t)
		for range 3 {
			sqlitetest.InsertChunk(t, db, "a real chunk of text")
		}

		embedAll(t, db, "bge-m3", 4)

		var countA int
		if err := db.QueryRow("SELECT count(*) FROM embeddings_bge_m3_4").Scan(&countA); err != nil {
			t.Fatalf("querying embeddings_bge_m3_4: %v", err)
		}
		if countA != 3 {
			t.Fatalf("got %d rows under bge-m3, want 3", countA)
		}

		// now bring a second model online, without touching bge-m3's table at all.
		second := embedAll(t, db, "qwen3-embedding_0_6b", 3)
		if second.calls != 1 {
			t.Fatalf("second model: EmbedChunks called %d times, want 1 (all 3 existing chunks in one batch)", second.calls)
		}

		var countB int
		if err := db.QueryRow("SELECT count(*) FROM embeddings_qwen3_embedding_0_6b_3").Scan(&countB); err != nil {
			t.Fatalf("querying embeddings_qwen3_embedding_0_6b_3: %v", err)
		}
		if countB != 3 {
			t.Fatalf("got %d rows under the second model, want 3 -- every existing chunk should have been backfilled", countB)
		}

		// bge-m3's table should be completely untouched by embedding the second model.
		if err := db.QueryRow("SELECT count(*) FROM embeddings_bge_m3_4").Scan(&countA); err != nil {
			t.Fatalf("re-querying embeddings_bge_m3_4: %v", err)
		}
		if countA != 3 {
			t.Fatalf("bge-m3 table changed after embedding a second model: now %d rows, want still 3", countA)
		}
	})
}
