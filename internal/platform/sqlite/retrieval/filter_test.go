package retrieval

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"
)

// insertChunkWithMetadata seeds a chunk in a source and sets the metadata_json of its document.
func insertChunkWithMetadata(t *testing.T, db *sql.DB, text, source, metadataJSON string) int64 {
	t.Helper()
	id := insertChunkInSource(t, db, text, source, 1.0)
	if _, err := db.Exec(`UPDATE documents SET metadata_json = ? WHERE id = (SELECT document_id FROM chunks WHERE id = ?)`, metadataJSON, id); err != nil {
		t.Fatalf("setting metadata_json: %v", err)
	}
	return id
}

func resultIDs(got []step.ScoredChunkID) []int64 {
	ids := make([]int64, 0, len(got))
	for _, g := range got {
		ids = append(ids, g.ID)
	}
	return ids
}

// filterFixture has 4 chunks that all contain "anchor", with different sources and metadata.
type filterFixture struct {
	db                         *sql.DB
	retriever                  *SQLiteRetriever
	core, nonCore, noKey, logs int64
}

func newFilterFixture(t *testing.T) filterFixture {
	t.Helper()
	db := sqlitetest.New(t)
	f := filterFixture{db: db}
	f.core = insertChunkWithMetadata(t, db, "anchor core", "manual", `{"product_core": true, "lang": "de", "level": 2}`)
	f.nonCore = insertChunkWithMetadata(t, db, "anchor noncore", "manual", `{"product_core": false, "lang": "en", "level": 3}`)
	f.noKey = insertChunkWithMetadata(t, db, "anchor nokey", "manual", `{"lang": "de"}`)
	f.logs = insertChunkWithMetadata(t, db, "anchor logs", "changelogs", `{"product_core": true}`)
	retriever, err := NewSQLiteRetriever(db, testLogger)
	if err != nil {
		t.Fatalf("NewSQLiteRetriever: %v", err)
	}
	f.retriever = retriever
	return f
}

func sortedIDs(ids ...int64) []int64 {
	slices.Sort(ids)
	return ids
}

func TestFilterFTS(t *testing.T) {
	f := newFilterFixture(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		filter step.Filter
		want   []int64
	}{
		{name: "no filter", filter: step.Filter{}, want: sortedIDs(f.core, f.nonCore, f.noKey, f.logs)},
		{name: "one source", filter: step.Filter{Sources: []string{"changelogs"}}, want: sortedIDs(f.logs)},
		{name: "two sources are OR", filter: step.Filter{Sources: []string{"manual", "changelogs"}}, want: sortedIDs(f.core, f.nonCore, f.noKey, f.logs)},
		{name: "bool false", filter: step.Filter{Metadata: map[string]any{"product_core": false}}, want: sortedIDs(f.nonCore)},
		{name: "bool true, a missing key does not match", filter: step.Filter{Metadata: map[string]any{"product_core": true}}, want: sortedIDs(f.core, f.logs)},
		{name: "string", filter: step.Filter{Metadata: map[string]any{"lang": "de"}}, want: sortedIDs(f.core, f.noKey)},
		{name: "number", filter: step.Filter{Metadata: map[string]any{"level": 3.0}}, want: sortedIDs(f.nonCore)},
		{name: "the string \"true\" is not the bool true", filter: step.Filter{Metadata: map[string]any{"product_core": "true"}}, want: sortedIDs()},
		{name: "keys are AND", filter: step.Filter{Metadata: map[string]any{"product_core": true, "lang": "de"}}, want: sortedIDs(f.core)},
		{name: "source and metadata", filter: step.Filter{Sources: []string{"manual"}, Metadata: map[string]any{"product_core": true}}, want: sortedIDs(f.core)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := f.retriever.TopKByFTS(ctx, "anchor", 10, tt.filter)
			if err != nil {
				t.Fatalf("TopKByFTS() error = %v", err)
			}
			if ids := sortedIDs(resultIDs(got)...); !slices.Equal(ids, tt.want) {
				t.Errorf("got ids %v, want %v", ids, tt.want)
			}
		})
	}
}

func TestFilterANN(t *testing.T) {
	f := newFilterFixture(t)
	ctx := context.Background()
	tableName, err := sqlite.SetupVecTable(ctx, f.db, 3, "bge-m3")
	if err != nil {
		t.Fatalf("SetupVecTable: %v", err)
	}
	seedEmbedding(t, f.db, tableName, f.core, []float64{1, 0, 0})
	seedEmbedding(t, f.db, tableName, f.nonCore, []float64{0.9, 0.1, 0})
	seedEmbedding(t, f.db, tableName, f.noKey, []float64{0, 1, 0})
	seedEmbedding(t, f.db, tableName, f.logs, []float64{-1, 0, 0}) // no two distances are equal, so the order is defined
	query := step.Query{Vector: []float64{1, 0, 0}, Dim: 3, Model: "bge-m3"}

	t.Run("a metadata filter returns only the matching chunks, ordered by distance", func(t *testing.T) {
		got, err := f.retriever.TopKByANN(ctx, query, 10, step.Filter{Metadata: map[string]any{"product_core": true}})
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		if ids := resultIDs(got); !slices.Equal(ids, []int64{f.core, f.logs}) {
			t.Errorf("got ids %v, want %v", ids, []int64{f.core, f.logs})
		}
	})

	t.Run("a narrow filter still finds its chunk outside the k nearest", func(t *testing.T) {
		// f.logs is the farthest chunk, a KNN with k=1 and a filter after it would return nothing
		got, err := f.retriever.TopKByANN(ctx, query, 1, step.Filter{Sources: []string{"changelogs"}})
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		if ids := resultIDs(got); !slices.Equal(ids, []int64{f.logs}) {
			t.Errorf("got ids %v, want %v", ids, []int64{f.logs})
		}
	})

	t.Run("a filter on all sources gives the same order and distances as no filter", func(t *testing.T) {
		unfiltered, err := f.retriever.TopKByANN(ctx, query, 10, step.Filter{})
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		filtered, err := f.retriever.TopKByANN(ctx, query, 10, step.Filter{Sources: []string{"manual", "changelogs"}})
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		if len(unfiltered) != 4 || len(filtered) != 4 {
			t.Fatalf("got %d and %d results, want 4 and 4", len(unfiltered), len(filtered))
		}
		for i := range unfiltered {
			if unfiltered[i].ID != filtered[i].ID || unfiltered[i].Score != filtered[i].Score {
				t.Errorf("rank %d: no filter %+v, filter %+v", i+1, unfiltered[i], filtered[i])
			}
		}
	})

	t.Run("a filter without a matching document gives no chunks", func(t *testing.T) {
		got, err := f.retriever.TopKByANN(ctx, query, 10, step.Filter{Metadata: map[string]any{"product_core": "no-such-value"}})
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d results, want 0", len(got))
		}
	})

	t.Run("chunks with the same distance come in chunk ID order", func(t *testing.T) {
		tieTable, err := sqlite.SetupVecTable(ctx, f.db, 3, "tie-model")
		if err != nil {
			t.Fatalf("SetupVecTable: %v", err)
		}
		// insert in ID order: for equal distances, the vec0 KNN gives the last inserted vector first
		want := sortedIDs(f.logs, f.noKey, f.nonCore, f.core)
		for _, id := range want {
			seedEmbedding(t, f.db, tieTable, id, []float64{0, 0, 1})
		}
		got, err := f.retriever.TopKByANN(ctx, step.Query{Vector: []float64{1, 0, 0}, Dim: 3, Model: "tie-model"}, 10, step.Filter{Sources: []string{"manual", "changelogs"}})
		if err != nil {
			t.Fatalf("TopKByANN() error = %v", err)
		}
		if gotIDs := resultIDs(got); !slices.Equal(gotIDs, want) {
			t.Errorf("got ids %v, want %v", gotIDs, want)
		}
	})
}
