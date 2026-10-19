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

// testFusion is the fusion of the example config. RunRetrieval has no defaults, the config validation guarantees the values.
var testFusion = FusionConfig{RRFK: 60, WeightFTS: 1, WeightANN: 1}

type fakeHydrator struct {
	chunks     []RetrievedChunk
	err        error
	calledWith []ScoredChunkID
	// weights, when set, makes the fake return one chunk per requested id, with the score
	// and rank of the id and the source weight from this map, like the real hydrator
	weights map[int64]float64
}

func (f *fakeHydrator) HydrateChunks(ctx context.Context, ids []ScoredChunkID) ([]RetrievedChunk, error) {
	f.calledWith = ids
	if f.err != nil {
		return nil, f.err
	}
	if f.weights != nil {
		chunks := make([]RetrievedChunk, 0, len(ids))
		for _, id := range ids {
			chunks = append(chunks, RetrievedChunk{ID: id.ID, Score: id.Score, Rank: id.Rank, SourceWeight: f.weights[id.ID]})
		}
		return chunks, nil
	}
	return f.chunks, nil
}

func scoredIDs(scored []ScoredChunkID) []int64 {
	out := make([]int64, 0, len(scored))
	for _, s := range scored {
		out = append(out, s.ID)
	}
	return out
}

func chunkIDs(chunks []RetrievedChunk) []int64 {
	ids := make([]int64, 0, len(chunks))
	for _, c := range chunks {
		ids = append(ids, c.ID)
	}
	return ids
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

func TestWeightedRRFMerge(t *testing.T) {
	fts := []ScoredChunkID{{ID: 1}, {ID: 2}}
	ann := []ScoredChunkID{{ID: 2}, {ID: 1}}

	// equal weights: both ids have the same score, tie-break by id
	got := weightedRRFMerge(60, []float64{1, 1}, fts, ann)
	if got[0].ID != 1 || got[0].Score != got[1].Score {
		t.Errorf("equal weights: got %+v, want a tie with id 1 first", got)
	}

	// ANN weight 2: the ANN rank 1 (id 2) wins
	w := 2.0
	got = weightedRRFMerge(60, []float64{1, w}, fts, ann)
	want2 := 1.0/float64(60+2) + w/float64(60+1)
	want1 := 1.0/float64(60+1) + w/float64(60+2)
	if got[0].ID != 2 || got[0].Score != want2 || got[1].Score != want1 {
		t.Errorf("ANN weight 2: got %+v, want id 2 first with %v, then %v", got, want2, want1)
	}
}

func TestSourceRerank(t *testing.T) {
	t.Run("weight multiplies the score and sets the rank", func(t *testing.T) {
		score, weight := 0.02, 0.5
		got := sourceRerank([]RetrievedChunk{{ID: 1, Score: score, Rank: 7, SourceWeight: weight}})
		want := []RetrievedChunk{{ID: 1, Score: score * weight, Rank: 1, SourceWeight: weight}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("sourceRerank() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("a lower score with a high enough weight outranks a higher score with a low weight", func(t *testing.T) {
		s1, w1 := 0.02, 1.0
		s2, w2 := 0.01, 5.0
		got := sourceRerank([]RetrievedChunk{
			{ID: 1, Score: s1, SourceWeight: w1},
			{ID: 2, Score: s2, SourceWeight: w2},
		})
		want := []RetrievedChunk{
			{ID: 2, Score: s2 * w2, Rank: 1, SourceWeight: w2},
			{ID: 1, Score: s1 * w1, Rank: 2, SourceWeight: w1},
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("sourceRerank() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("equal weighted scores tie-break by ascending id", func(t *testing.T) {
		score, weight := 0.03, 1.0
		got := sourceRerank([]RetrievedChunk{
			{ID: 20, Score: score, SourceWeight: weight},
			{ID: 10, Score: score, SourceWeight: weight},
		})
		if diff := cmp.Diff([]int64{10, 20}, chunkIDs(got)); diff != "" {
			t.Errorf("sourceRerank() order mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("does not change the input slice", func(t *testing.T) {
		in := []RetrievedChunk{{ID: 1, Score: 0.02, SourceWeight: 0.5}}
		sourceRerank(in)
		if in[0].Score != 0.02 {
			t.Errorf("input score = %v, want 0.02 unchanged", in[0].Score)
		}
	})
}

func TestRunHybrid(t *testing.T) {
	t.Run("fts and ann are both queried with the pool size, merged, and cut to the pool size", func(t *testing.T) {
		retriever := &fakeRetriever{
			ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}, {ID: 3}},
			annIDs: []ScoredChunkID{{ID: 2}, {ID: 3}, {ID: 4}},
		}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}

		ids, err := runHybrid(testCtx, "hello", 2, testFusion, client, retriever)
		if err != nil {
			t.Fatalf("runHybrid() error = %v", err)
		}
		// 2 and 3 are in both lists, so they have the highest fused scores
		want := []int64{2, 3}
		got := []int64{ids[0].ID, ids[1].ID}
		if len(ids) != 2 || cmp.Diff(want, got) != "" {
			t.Fatalf("got ids %+v, want %v (cut to the pool size)", ids, want)
		}

		if len(retriever.ftsCalls) != 1 || retriever.ftsKCalls[0] != 2 {
			t.Errorf("fts should be called once with k=2 (the pool size), got calls=%d k=%v", len(retriever.ftsCalls), retriever.ftsKCalls)
		}
		if len(retriever.annCalls) != 1 || retriever.annKCalls[0] != 2 {
			t.Errorf("ann should be called once with k=2 (the pool size), got calls=%d k=%v", len(retriever.annCalls), retriever.annKCalls)
		}
	})

	t.Run("returns the merged list when it is smaller than the pool", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}

		ids, err := runHybrid(testCtx, "hello", 10, testFusion, client, retriever)
		if err != nil {
			t.Fatalf("runHybrid() error = %v", err)
		}
		if len(ids) != 2 {
			t.Fatalf("got %d ids, want 2", len(ids))
		}
	})
}

func TestRunRetrieval(t *testing.T) {
	t.Run("FTS strategy skips embedding and source weighting entirely", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}}}
		client := &fakeEmbedClient{err: errors.New("should never be called")}
		hydrator := &fakeHydrator{chunks: []RetrievedChunk{{ID: 1, Rank: 1}}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer, Fusion: testFusion}
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

	t.Run("Hybrid strategy hydrates the pool, applies the source weight, then cuts to k", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}, {ID: 3}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		hydrator := &fakeHydrator{weights: map[int64]float64{1: 1.0, 2: 1.0, 3: 5.0}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer, Fusion: testFusion, PoolSize: 3}
		chunks, err := RunRetrieval(testCtx, "q", 2, StrategyHybrid, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if len(hydrator.calledWith) != 3 {
			t.Fatalf("hydrator called with %d ids, want 3 (the full pool)", len(hydrator.calledWith))
		}
		// chunk 3 has the lowest fused score (rank 3) but a 5x source weight
		if diff := cmp.Diff([]int64{3, 1}, chunkIDs(chunks)); diff != "" {
			t.Errorf("chunk order mismatch (-want +got):\n%s", diff)
		}
		if chunks[0].Rank != 1 || chunks[1].Rank != 2 {
			t.Errorf("ranks = %d, %d, want 1, 2", chunks[0].Rank, chunks[1].Rank)
		}
	})

	t.Run("Hybrid strategy does not weigh a chunk outside the pool", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}, {ID: 3}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		hydrator := &fakeHydrator{weights: map[int64]float64{1: 1.0, 2: 1.0, 3: 5.0}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer, Fusion: testFusion, PoolSize: 2}
		chunks, err := RunRetrieval(testCtx, "q", 1, StrategyHybrid, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if diff := cmp.Diff([]int64{1, 2}, scoredIDs(hydrator.calledWith)); diff != "" {
			t.Errorf("hydrated ids mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff([]int64{1}, chunkIDs(chunks)); diff != "" {
			t.Errorf("chunk 3 is outside the pool and must not win with its weight (-want +got):\n%s", diff)
		}
	})

	t.Run("Hybrid strategy uses k as the pool when the pool size is smaller than k", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}, {ID: 3}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		hydrator := &fakeHydrator{weights: map[int64]float64{1: 1.0, 2: 1.0, 3: 1.0}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer, Fusion: testFusion, PoolSize: 1}
		chunks, err := RunRetrieval(testCtx, "q", 3, StrategyHybrid, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if retriever.ftsKCalls[0] != 3 || len(chunks) != 3 {
			t.Errorf("fts k = %v, chunks = %d, want 3 and 3", retriever.ftsKCalls, len(chunks))
		}
	})

	t.Run("Hybrid strategy returns fewer than k chunks without panicking when the pool is smaller than k", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: []ScoredChunkID{{ID: 1}, {ID: 2}}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		hydrator := &fakeHydrator{weights: map[int64]float64{1: 1.0, 2: 1.0}}
		renderer := &fakeRenderer{}

		deps := RetrievalDeps{Logger: testLogger, Hydrator: hydrator, Retriever: retriever, EmbeddingsClient: client, Renderer: renderer, Fusion: testFusion, PoolSize: 40}
		chunks, err := RunRetrieval(testCtx, "q", 10, StrategyHybrid, deps)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if len(chunks) != 2 {
			t.Fatalf("got %d chunks, want 2", len(chunks))
		}
	})
}
