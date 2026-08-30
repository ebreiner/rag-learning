package step

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var (
	testCtx    = context.Background()
	testLogger = slog.New(slog.DiscardHandler)
)

type fakeRetriever struct {
	annIDs    []ScoredChunkID
	annErr    error
	annCalls  []Query
	annKCalls []int64
	ftsIDs    []ScoredChunkID
	ftsErr    error
	ftsCalls  []string
	ftsKCalls []int64
}

func (f *fakeRetriever) TopKByANN(ctx context.Context, query Query, k int64) ([]ScoredChunkID, error) {
	f.annCalls = append(f.annCalls, query)
	f.annKCalls = append(f.annKCalls, k)
	if f.annErr != nil {
		return nil, f.annErr
	}
	return f.annIDs, nil
}

func (f *fakeRetriever) TopKByFTS(ctx context.Context, query string, k int64) ([]ScoredChunkID, error) {
	f.ftsCalls = append(f.ftsCalls, query)
	f.ftsKCalls = append(f.ftsKCalls, k)
	if f.ftsErr != nil {
		return nil, f.ftsErr
	}
	return f.ftsIDs, nil
}

type fakeEmbedClient struct {
	query Query
	err   error
}

func (f *fakeEmbedClient) EmbedQuery(ctx context.Context, q string) (Query, error) {
	if f.err != nil {
		return Query{}, f.err
	}
	return f.query, nil
}

type fakeCollWeigher struct {
	weights map[int64]float64
	err     error
}

func (f *fakeCollWeigher) WeighChunks(ctx context.Context, chunkIDs []ScoredChunkID) (map[int64]float64, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.weights, nil
}

type fakeHydrator struct {
	chunks     []RetrievedChunk
	err        error
	calledWith []ScoredChunkID
}

func (f *fakeHydrator) HydrateChunks(ctx context.Context, ids []ScoredChunkID) ([]RetrievedChunk, error) {
	f.calledWith = ids
	if f.err != nil {
		return nil, f.err
	}
	return f.chunks, nil
}

type fakeRenderer struct {
	err error
}

func (f *fakeRenderer) RenderChunks(ctx context.Context, chunks []RetrievedChunk) ([]RetrievedChunk, error) {
	if f.err != nil {
		return nil, f.err
	}
	return chunks, nil
}

func TestRunANN(t *testing.T) {
	t.Run("embeds the query then searches with the embedded vector", func(t *testing.T) {
		wantQuery := Query{Vector: []float64{0.1, 0.2}, Dim: 2, Model: "fake"}
		retriever := &fakeRetriever{annIDs: []ScoredChunkID{{ID: 3}, {ID: 1}, {ID: 2}}}
		client := &fakeEmbedClient{query: wantQuery}

		ids, err := runANN(testCtx, "hello", 10, client, retriever)
		if err != nil {
			t.Fatalf("runANN() error = %v", err)
		}
		want := []ScoredChunkID{{ID: 3, Rank: 1}, {ID: 1, Rank: 2}, {ID: 2, Rank: 3}}
		if diff := cmp.Diff(want, ids); diff != "" {
			t.Errorf("ids mismatch (-want +got):\n%s", diff)
		}
		if len(retriever.annCalls) != 1 {
			t.Fatalf("TopKByANN called %d times, want 1", len(retriever.annCalls))
		}
		if diff := cmp.Diff(wantQuery, retriever.annCalls[0]); diff != "" {
			t.Errorf("TopKByANN called with wrong query (-want +got):\n%s", diff)
		}
	})

	t.Run("embed error propagates without calling the retriever", func(t *testing.T) {
		wantErr := errors.New("embed broke")
		retriever := &fakeRetriever{}
		client := &fakeEmbedClient{err: wantErr}

		_, err := runANN(testCtx, "hello", 10, client, retriever)
		if !errors.Is(err, wantErr) {
			t.Fatalf("runANN() error = %v, want %v", err, wantErr)
		}
		if len(retriever.annCalls) != 0 {
			t.Errorf("TopKByANN should not have been called after an embed error")
		}
	})

	t.Run("retriever error propagates", func(t *testing.T) {
		wantErr := errors.New("ann broke")
		retriever := &fakeRetriever{annErr: wantErr}
		client := &fakeEmbedClient{}

		_, err := runANN(testCtx, "hello", 10, client, retriever)
		if !errors.Is(err, wantErr) {
			t.Fatalf("runANN() error = %v, want %v", err, wantErr)
		}
	})
}

func TestRunFTS(t *testing.T) {
	retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 5}, {ID: 6}}}
	ids, err := runFTS(testCtx, "hello", 10, retriever)
	if err != nil {
		t.Fatalf("runFTS() error = %v", err)
	}
	want := []ScoredChunkID{{ID: 5, Rank: 1}, {ID: 6, Rank: 2}}
	if diff := cmp.Diff(want, ids); diff != "" {
		t.Errorf("ids mismatch (-want +got):\n%s", diff)
	}
	if len(retriever.ftsCalls) != 1 || retriever.ftsCalls[0] != "hello" {
		t.Errorf("TopKByFTS called with %v, want [hello]", retriever.ftsCalls)
	}
}

// rrfScore mirrors rrfMerge's own accumulation (scores[id] += 1.0/float64(rrfK+rank))
// at runtime rather than as a compile-time constant expression -- a literal
// like `1.0/61 + 1.0/62` gets folded by the compiler using exact arbitrary-
// precision arithmetic and rounds to float64 once, while production divides
// and rounds each term separately at runtime, which can land 1 ULP away.
func rrfScore(ranks ...int64) float64 {
	var s float64
	for _, r := range ranks {
		s += 1.0 / float64(rrfK+r)
	}
	return s
}

func TestRrfMerge(t *testing.T) {
	tests := []struct {
		name     string
		rankings [][]ScoredChunkID
		want     []ScoredChunkID
	}{
		{
			name:     "single ranking preserves order",
			rankings: [][]ScoredChunkID{{{ID: 1}, {ID: 2}, {ID: 3}}},
			want: []ScoredChunkID{
				{ID: 1, Score: rrfScore(1), Rank: 1},
				{ID: 2, Score: rrfScore(2), Rank: 2},
				{ID: 3, Score: rrfScore(3), Rank: 3},
			},
		},
		{
			name: "chunk appearing in both rankings outranks one appearing in only one",
			rankings: [][]ScoredChunkID{
				{{ID: 10}, {ID: 1}, {ID: 2}},
				{{ID: 20}, {ID: 1}, {ID: 3}},
			},
			// 1 appears at a good rank in both lists, so it should win overall;
			// rrfMerge no longer truncates, so every unique id from both
			// rankings comes back, not just the top few.
			want: []ScoredChunkID{
				{ID: 1, Score: rrfScore(2, 2), Rank: 1},
				{ID: 10, Score: rrfScore(1), Rank: 2},
				{ID: 20, Score: rrfScore(1), Rank: 3},
				{ID: 2, Score: rrfScore(3), Rank: 4},
				{ID: 3, Score: rrfScore(3), Rank: 5},
			},
		},
		{
			name:     "empty rankings produce an empty result",
			rankings: [][]ScoredChunkID{},
			want:     []ScoredChunkID{},
		},
		{
			name: "equal scores tie-break by ascending chunk id",
			rankings: [][]ScoredChunkID{
				{{ID: 20}},
				{{ID: 10}},
			},
			want: []ScoredChunkID{
				{ID: 10, Score: rrfScore(1), Rank: 1},
				{ID: 20, Score: rrfScore(1), Rank: 2},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rrfMerge(tt.rankings...)
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("rrfMerge() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestCollectionRerank(t *testing.T) {
	t.Run("weight multiplies the raw score", func(t *testing.T) {
		score, weight := 0.02, 0.5
		got := collectionRerank(testCtx, []ScoredChunkID{{ID: 1, Score: score}}, map[int64]float64{1: weight}, testLogger)
		want := []ScoredChunkID{{ID: 1, Score: score * weight, Rank: 1}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("collectionRerank() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("a lower raw score with a high enough weight outranks a higher raw score with a low weight", func(t *testing.T) {
		s1, w1 := 0.02, 1.0
		s2, w2 := 0.01, 5.0
		got := collectionRerank(
			testCtx,
			[]ScoredChunkID{{ID: 1, Score: s1}, {ID: 2, Score: s2}},
			map[int64]float64{1: w1, 2: w2},
			testLogger,
		)
		want := []ScoredChunkID{
			{ID: 2, Score: s2 * w2, Rank: 1},
			{ID: 1, Score: s1 * w1, Rank: 2},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("collectionRerank() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("equal boosted scores tie-break by ascending id", func(t *testing.T) {
		score, weight := 0.03, 1.0
		got := collectionRerank(
			testCtx,
			[]ScoredChunkID{{ID: 20, Score: score}, {ID: 10, Score: score}},
			map[int64]float64{20: weight, 10: weight},
			testLogger,
		)
		want := []ScoredChunkID{
			{ID: 10, Score: score * weight, Rank: 1},
			{ID: 20, Score: score * weight, Rank: 2},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("collectionRerank() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("a chunk with no discoverable weight is dropped, not defaulted", func(t *testing.T) {
		got := collectionRerank(
			testCtx,
			[]ScoredChunkID{{ID: 1, Score: 0.02}, {ID: 2, Score: 0.01}},
			map[int64]float64{1: 1.0}, // no entry for id 2
			testLogger,
		)
		want := []ScoredChunkID{{ID: 1, Score: 0.02, Rank: 1}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("collectionRerank() mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestRunHybrid(t *testing.T) {
	t.Run("fts and ann are both queried at k*4, merged, weighted, and truncated to k", func(t *testing.T) {
		retriever := &fakeRetriever{
			ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}, {ID: 3}},
			annIDs: []ScoredChunkID{{ID: 2}, {ID: 3}, {ID: 4}},
		}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		weigher := &fakeCollWeigher{weights: map[int64]float64{1: 1.0, 2: 1.0, 3: 5.0, 4: 1.0}}

		ids, err := runHybrid(testCtx, "hello", 2, client, retriever, weigher, testLogger)
		if err != nil {
			t.Fatalf("runHybrid() error = %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("got %d ids, want 2 (truncated to k)", len(ids))
		}
		// chunk 3 has a modest raw RRF score but a 5x collection weight, so it
		// should be boosted to the top despite not being the strongest raw match.
		if ids[0].ID != 3 {
			t.Errorf("ids[0].ID = %d, want 3 (boosted by collection weight)", ids[0].ID)
		}

		if len(retriever.ftsCalls) != 1 || retriever.ftsKCalls[0] != 8 {
			t.Errorf("fts should be called once with k=8 (k*4), got calls=%d k=%v", len(retriever.ftsCalls), retriever.ftsKCalls)
		}
		if len(retriever.annCalls) != 1 || retriever.annKCalls[0] != 8 {
			t.Errorf("ann should be called once with k=8 (k*4), got calls=%d k=%v", len(retriever.annCalls), retriever.annKCalls)
		}
	})

	t.Run("weigher error propagates", func(t *testing.T) {
		wantErr := errors.New("weigh broke")
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		weigher := &fakeCollWeigher{err: wantErr}

		_, err := runHybrid(testCtx, "hello", 2, client, retriever, weigher, testLogger)
		if !errors.Is(err, wantErr) {
			t.Fatalf("runHybrid() error = %v, want %v", err, wantErr)
		}
	})

	// regression guard: the merged+weighted pool can legitimately end up
	// smaller than k (small corpus, narrow query, or chunks dropped by
	// collectionRerank for missing a collection weight) -- runHybrid must not
	// assume there are at least k survivors before slicing.
	t.Run("does not panic when the weighted pool is smaller than k", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		weigher := &fakeCollWeigher{weights: map[int64]float64{1: 1.0, 2: 1.0}}

		ids, err := runHybrid(testCtx, "hello", 10, client, retriever, weigher, testLogger)
		if err != nil {
			t.Fatalf("runHybrid() error = %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("got %d ids, want 2", len(ids))
		}
	})
}

func TestRunRetrieval(t *testing.T) {
	t.Run("FTS strategy skips embedding and collection weighting entirely", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}}}
		client := &fakeEmbedClient{err: errors.New("should never be called")}
		weigher := &fakeCollWeigher{err: errors.New("should never be called")}
		hydrator := &fakeHydrator{chunks: []RetrievedChunk{{ID: 1, Rank: 1}}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, CollWeigher: weigher, Renderer: renderer}
		chunks, err := RunRetrieval(testCtx, "q", 5, StrategyFTS, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if len(chunks) != 1 {
			t.Fatalf("got %d chunks, want 1", len(chunks))
		}
		want := []ScoredChunkID{{ID: 1, Rank: 1}}
		if diff := cmp.Diff(want, hydrator.calledWith); diff != "" {
			t.Errorf("hydrator called with wrong ids (-want +got):\n%s", diff)
		}
	})

	t.Run("ANN strategy uses runANN", func(t *testing.T) {
		retriever := &fakeRetriever{annIDs: []ScoredChunkID{{ID: 7}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		hydrator := &fakeHydrator{}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer}
		_, err := RunRetrieval(testCtx, "q", 5, StrategyANN, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		want := []ScoredChunkID{{ID: 7, Rank: 1}}
		if diff := cmp.Diff(want, hydrator.calledWith); diff != "" {
			t.Errorf("hydrator called with wrong ids (-want +got):\n%s", diff)
		}
	})

	t.Run("unknown strategy errors before touching the hydrator", func(t *testing.T) {
		retriever := &fakeRetriever{}
		client := &fakeEmbedClient{}
		hydrator := &fakeHydrator{}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer}
		_, err := RunRetrieval(testCtx, "q", 5, RetrievalStrategy("bogus"), deps)
		if err == nil {
			t.Fatalf("expected an error for an unknown strategy")
		}
		if hydrator.calledWith != nil {
			t.Errorf("hydrator should not have been called")
		}
	})

	t.Run("hydrator error propagates", func(t *testing.T) {
		wantErr := errors.New("hydrate broke")
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}}}
		client := &fakeEmbedClient{}
		hydrator := &fakeHydrator{err: wantErr}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer}
		_, err := RunRetrieval(testCtx, "q", 5, StrategyFTS, deps)
		if !errors.Is(err, wantErr) {
			t.Fatalf("RunRetrieval() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("renderer error propagates", func(t *testing.T) {
		wantErr := errors.New("render broke")
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}}}
		client := &fakeEmbedClient{}
		hydrator := &fakeHydrator{chunks: []RetrievedChunk{{ID: 1}}}
		renderer := &fakeRenderer{err: wantErr}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer}
		_, err := RunRetrieval(testCtx, "q", 5, StrategyFTS, deps)
		if !errors.Is(err, wantErr) {
			t.Fatalf("RunRetrieval() error = %v, want %v", err, wantErr)
		}
	})

	t.Run("Hybrid strategy weighs, reranks, and truncates before ever calling the hydrator", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}, {ID: 3}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		weigher := &fakeCollWeigher{weights: map[int64]float64{1: 1.0, 2: 1.0, 3: 5.0}}
		hydrator := &fakeHydrator{chunks: []RetrievedChunk{{ID: 3}, {ID: 1}}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, CollWeigher: weigher, Renderer: renderer}
		chunks, err := RunRetrieval(testCtx, "q", 2, StrategyHybrid, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}
		if len(hydrator.calledWith) != 2 {
			t.Fatalf("hydrator called with %d ids, want 2 (already truncated to k)", len(hydrator.calledWith))
		}
		// chunk 3 has the lowest raw RRF score (rank 3) but a 5x collection
		// weight, so it should be the one surviving truncation ahead of chunk 2.
		if hydrator.calledWith[0].ID != 3 {
			t.Errorf("hydrator.calledWith[0].ID = %d, want 3 (boosted by collection weight)", hydrator.calledWith[0].ID)
		}
	})

	t.Run("Hybrid strategy returns fewer than k chunks without panicking when the pool is smaller than k", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		weigher := &fakeCollWeigher{weights: map[int64]float64{1: 1.0, 2: 1.0}}
		hydrator := &fakeHydrator{chunks: []RetrievedChunk{{ID: 1}, {ID: 2}}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, CollWeigher: weigher, Renderer: renderer}
		chunks, err := RunRetrieval(testCtx, "q", 10, StrategyHybrid, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}
	})
}
