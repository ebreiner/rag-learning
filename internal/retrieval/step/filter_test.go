package step

import (
	"errors"
	"testing"
)

func TestFilterValidate(t *testing.T) {
	known := []string{"manual", "changelogs"}
	tests := []struct {
		name    string
		filter  Filter
		wantErr bool
	}{
		{name: "empty filter", filter: Filter{}},
		{name: "known sources", filter: Filter{Sources: []string{"manual", "changelogs"}}},
		{name: "unknown source", filter: Filter{Sources: []string{"manual", "nope"}}, wantErr: true},
		{name: "bool, string and number values", filter: Filter{Metadata: map[string]any{"product_core": false, "lang": "de", "level": 2.0, "n": 3}}},
		{name: "key with dot and dash", filter: Filter{Metadata: map[string]any{"classification.personal_data": true, "a-b": "x"}}},
		{name: "key with a quote", filter: Filter{Metadata: map[string]any{`a"b`: true}}, wantErr: true},
		{name: "key with a space", filter: Filter{Metadata: map[string]any{"a b": true}}, wantErr: true},
		{name: "empty key", filter: Filter{Metadata: map[string]any{"": true}}, wantErr: true},
		{name: "list value", filter: Filter{Metadata: map[string]any{"tags": []any{"a"}}}, wantErr: true},
		{name: "object value", filter: Filter{Metadata: map[string]any{"o": map[string]any{}}}, wantErr: true},
		{name: "null value", filter: Filter{Metadata: map[string]any{"k": nil}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.validate(known)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalidFilter) {
				t.Errorf("validate() error = %v, want ErrInvalidFilter", err)
			}
		})
	}
}

func TestRunRetrievalFilter(t *testing.T) {
	t.Run("an invalid filter fails before the retriever", func(t *testing.T) {
		retriever := &fakeRetriever{}
		deps := RetrievalDeps{Logger: testLogger, Retriever: retriever, Sources: []string{"manual"}}

		_, err := RunRetrieval(testCtx, "q", 5, StrategyFTS, Filter{Sources: []string{"nope"}}, deps)
		if !errors.Is(err, ErrInvalidFilter) {
			t.Fatalf("RunRetrieval() error = %v, want ErrInvalidFilter", err)
		}
		if len(retriever.ftsCalls) != 0 {
			t.Errorf("retriever called %d times, want 0", len(retriever.ftsCalls))
		}
	})

	t.Run("hybrid gives the filter to FTS and ANN", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		hydrator := &fakeHydrator{weights: map[int64]float64{1: 1.0}}
		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client,
			Renderer: &fakeRenderer{}, Fusion: testFusion, PoolSize: 40, Sources: []string{"manual"}}
		filter := Filter{Sources: []string{"manual"}, Metadata: map[string]any{"product_core": false}}

		if _, err := RunRetrieval(testCtx, "q", 5, StrategyHybrid, filter, deps); err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if len(retriever.filters) != 2 {
			t.Fatalf("got %d retriever calls, want 2 (FTS and ANN)", len(retriever.filters))
		}
		for i, got := range retriever.filters {
			if len(got.Sources) != 1 || got.Sources[0] != "manual" || got.Metadata["product_core"] != false {
				t.Errorf("call %d got filter %+v, want %+v", i, got, filter)
			}
		}
	})
}
