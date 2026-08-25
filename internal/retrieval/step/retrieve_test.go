package step

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var testCtx = context.Background()

type fakeRetriever struct {
	annIDs    RetrievedChunkIDs
	annErr    error
	annCalls  []Query
	annKCalls []int64
	ftsIDs    RetrievedChunkIDs
	ftsErr    error
	ftsCalls  []string
	ftsKCalls []int64
}

func (f *fakeRetriever) TopKByANN(ctx context.Context, query Query, k int64) (RetrievedChunkIDs, error) {
	f.annCalls = append(f.annCalls, query)
	f.annKCalls = append(f.annKCalls, k)
	if f.annErr != nil {
		return nil, f.annErr
	}
	return f.annIDs, nil
}

func (f *fakeRetriever) TopKByFTS(ctx context.Context, query string, k int64) (RetrievedChunkIDs, error) {
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

type fakeHydrator struct {
	chunks     []RetrievedChunk
	err        error
	calledWith RetrievedChunkIDs
}

func (f *fakeHydrator) HydrateChunks(ctx context.Context, ids RetrievedChunkIDs) ([]RetrievedChunk, error) {
	f.calledWith = ids
	if f.err != nil {
		return nil, f.err
	}
	return f.chunks, nil
}

func TestAnn(t *testing.T) {
	t.Run("embeds the query then searches with the embedded vector", func(t *testing.T) {
		wantQuery := Query{Vector: []float64{0.1, 0.2}, Dim: 2, Model: "fake"}
		retriever := &fakeRetriever{annIDs: RetrievedChunkIDs{3, 1, 2}}
		client := &fakeEmbedClient{query: wantQuery}

		ids, err := ann(testCtx, "hello", 10, client, retriever)
		if err != nil {
			t.Fatalf("ann() error = %v", err)
		}
		if diff := cmp.Diff(RetrievedChunkIDs{3, 1, 2}, ids); diff != "" {
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

		_, err := ann(testCtx, "hello", 10, client, retriever)
		if !errors.Is(err, wantErr) {
			t.Fatalf("ann() error = %v, want %v", err, wantErr)
		}
		if len(retriever.annCalls) != 0 {
			t.Errorf("TopKByANN should not have been called after an embed error")
		}
	})

	t.Run("retriever error propagates", func(t *testing.T) {
		wantErr := errors.New("ann broke")
		retriever := &fakeRetriever{annErr: wantErr}
		client := &fakeEmbedClient{}

		_, err := ann(testCtx, "hello", 10, client, retriever)
		if !errors.Is(err, wantErr) {
			t.Fatalf("ann() error = %v, want %v", err, wantErr)
		}
	})
}

func TestFts(t *testing.T) {
	retriever := &fakeRetriever{ftsIDs: RetrievedChunkIDs{5, 6}}
	ids, err := fts(testCtx, "hello", 10, retriever)
	if err != nil {
		t.Fatalf("fts() error = %v", err)
	}
	if diff := cmp.Diff(RetrievedChunkIDs{5, 6}, ids); diff != "" {
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

func TestHybrid(t *testing.T) {
	retriever := &fakeRetriever{
		ftsIDs: RetrievedChunkIDs{1, 2, 3},
		annIDs: RetrievedChunkIDs{2, 3, 4},
	}
	client := &fakeEmbedClient{query: Query{Model: "fake"}}

	ids, err := hybrid(testCtx, "hello", 10, retriever, client)
	if err != nil {
		t.Fatalf("hybrid() error = %v", err)
	}

	// chunk 2 and 3 appear in both rankings, so RRF should rank them above
	// chunks that only appear in one ranking -- computed by hand from rrfK=60:
	// 2: 1/61 (ann rank1) + 1/62 (fts rank2) ; 3: 1/62 (ann rank2) + 1/63 (fts rank3)
	// 1: 1/61 (fts rank1) only ; 4: 1/63 (ann rank3) only
	want := []scoredChunkID{
		{ID: 2, Score: rrfScore(1, 2)},
		{ID: 3, Score: rrfScore(2, 3)},
		{ID: 1, Score: rrfScore(1)},
		{ID: 4, Score: rrfScore(3)},
	}
	if diff := cmp.Diff(want, ids); diff != "" {
		t.Errorf("merged ids mismatch (-want +got):\n%s", diff)
	}

	if len(retriever.ftsCalls) != 1 {
		t.Fatalf("fts should be called exactly once, got %d", len(retriever.ftsCalls))
	}
	if len(retriever.annCalls) != 1 {
		t.Fatalf("ann should be called exactly once, got %d", len(retriever.annCalls))
	}

	// hybrid() scales k by 4 before querying either ranking -- assert it
	// actually happens rather than just trusting the comment.
	wantHybridK := int64(40)
	if retriever.ftsKCalls[0] != wantHybridK {
		t.Errorf("fts called with k=%d, want %d", retriever.ftsKCalls[0], wantHybridK)
	}
	if retriever.annKCalls[0] != wantHybridK {
		t.Errorf("ann called with k=%d, want %d", retriever.annKCalls[0], wantHybridK)
	}
}

func TestRrfMerge(t *testing.T) {
	tests := []struct {
		name     string
		rankings []RetrievedChunkIDs
		want     []scoredChunkID
	}{
		{
			name:     "single ranking preserves order",
			rankings: []RetrievedChunkIDs{{1, 2, 3}},
			want: []scoredChunkID{
				{ID: 1, Score: rrfScore(1)},
				{ID: 2, Score: rrfScore(2)},
				{ID: 3, Score: rrfScore(3)},
			},
		},
		{
			name: "chunk appearing in both rankings outranks one appearing in only one",
			rankings: []RetrievedChunkIDs{
				{10, 1, 2},
				{20, 1, 3},
			},
			// 1 appears at a good rank in both lists, so it should win overall;
			// rrfMerge no longer truncates, so every unique id from both
			// rankings comes back, not just the top few.
			want: []scoredChunkID{
				{ID: 1, Score: rrfScore(2, 2)},
				{ID: 10, Score: rrfScore(1)},
				{ID: 20, Score: rrfScore(1)},
				{ID: 2, Score: rrfScore(3)},
				{ID: 3, Score: rrfScore(3)},
			},
		},
		{
			name:     "empty rankings produce an empty result",
			rankings: []RetrievedChunkIDs{},
			want:     []scoredChunkID{},
		},
		{
			name: "equal scores tie-break by ascending chunk id",
			rankings: []RetrievedChunkIDs{
				{20},
				{10},
			},
			want: []scoredChunkID{
				{ID: 10, Score: rrfScore(1)},
				{ID: 20, Score: rrfScore(1)},
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

func TestRunRetrieval(t *testing.T) {
	t.Run("FTS strategy skips embedding entirely", func(t *testing.T) {
		retriever := &fakeRetriever{ftsIDs: RetrievedChunkIDs{1}}
		client := &fakeEmbedClient{err: errors.New("should never be called")}
		hydrator := &fakeHydrator{chunks: []RetrievedChunk{{Rank: 1}}}

		chunks, err := RunRetrieval(testCtx, "q", FTS, 5, hydrator, retriever, client)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if len(chunks) != 1 {
			t.Fatalf("got %d chunks, want 1", len(chunks))
		}
		if diff := cmp.Diff(RetrievedChunkIDs{1}, hydrator.calledWith); diff != "" {
			t.Errorf("hydrator called with wrong ids (-want +got):\n%s", diff)
		}
	})

	t.Run("Embedding strategy uses ann", func(t *testing.T) {
		retriever := &fakeRetriever{annIDs: RetrievedChunkIDs{7}}
		client := &fakeEmbedClient{query: Query{Model: "fake"}}
		hydrator := &fakeHydrator{}

		_, err := RunRetrieval(testCtx, "q", Embedding, 5, hydrator, retriever, client)
		if err != nil {
			t.Fatalf("RunRetrieval() error = %v", err)
		}
		if diff := cmp.Diff(RetrievedChunkIDs{7}, hydrator.calledWith); diff != "" {
			t.Errorf("hydrator called with wrong ids (-want +got):\n%s", diff)
		}
	})

	t.Run("unknown strategy errors before touching the hydrator", func(t *testing.T) {
		retriever := &fakeRetriever{}
		client := &fakeEmbedClient{}
		hydrator := &fakeHydrator{}

		_, err := RunRetrieval(testCtx, "q", RetrievalStrategy("bogus"), 5, hydrator, retriever, client)
		if err == nil {
			t.Fatalf("expected an error for an unknown strategy")
		}
		if hydrator.calledWith != nil {
			t.Errorf("hydrator should not have been called")
		}
	})

	t.Run("hydrator error propagates", func(t *testing.T) {
		wantErr := errors.New("hydrate broke")
		retriever := &fakeRetriever{ftsIDs: RetrievedChunkIDs{1}}
		client := &fakeEmbedClient{}
		hydrator := &fakeHydrator{err: wantErr}

		_, err := RunRetrieval(testCtx, "q", FTS, 5, hydrator, retriever, client)
		if !errors.Is(err, wantErr) {
			t.Fatalf("RunRetrieval() error = %v, want %v", err, wantErr)
		}
	})
}
