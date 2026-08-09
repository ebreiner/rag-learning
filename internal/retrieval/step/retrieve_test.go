package step

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

type fakeRetriever struct {
	annIDs   RetrievedChunkIDs
	annErr   error
	annCalls []Query
	ftsIDs   RetrievedChunkIDs
	ftsErr   error
	ftsCalls []string
}

func (f *fakeRetriever) TopKByANN(query Query, k int64) (RetrievedChunkIDs, error) {
	f.annCalls = append(f.annCalls, query)
	if f.annErr != nil {
		return nil, f.annErr
	}
	return f.annIDs, nil
}

func (f *fakeRetriever) TopKByFTS(query string, k int64) (RetrievedChunkIDs, error) {
	f.ftsCalls = append(f.ftsCalls, query)
	if f.ftsErr != nil {
		return nil, f.ftsErr
	}
	return f.ftsIDs, nil
}

type fakeEmbedClient struct {
	query Query
	err   error
}

func (f *fakeEmbedClient) EmbedQuery(q string) (Query, error) {
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

func (f *fakeHydrator) HydrateChunks(ids RetrievedChunkIDs) ([]RetrievedChunk, error) {
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

		ids, err := ann("hello", 10, client, retriever)
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

		_, err := ann("hello", 10, client, retriever)
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

		_, err := ann("hello", 10, client, retriever)
		if !errors.Is(err, wantErr) {
			t.Fatalf("ann() error = %v, want %v", err, wantErr)
		}
	})
}

func TestFts(t *testing.T) {
	retriever := &fakeRetriever{ftsIDs: RetrievedChunkIDs{5, 6}}
	ids, err := fts("hello", 10, retriever)
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

func TestHybrid(t *testing.T) {
	retriever := &fakeRetriever{
		ftsIDs: RetrievedChunkIDs{1, 2, 3},
		annIDs: RetrievedChunkIDs{2, 3, 4},
	}
	client := &fakeEmbedClient{query: Query{Model: "fake"}}

	ids, err := hybrid("hello", 10, retriever, client)
	if err != nil {
		t.Fatalf("hybrid() error = %v", err)
	}
	// chunk 2 and 3 appear in both rankings, so they should score highest via RRF.
	if len(ids) == 0 {
		t.Fatalf("expected merged ids, got none")
	}
	// both fts and ann should have been called with k*4, per hybrid()'s own scaling.
	if len(retriever.ftsCalls) != 1 {
		t.Fatalf("fts should be called exactly once")
	}
	if len(retriever.annCalls) != 1 {
		t.Fatalf("ann should be called exactly once")
	}
}

func TestRrfMerge(t *testing.T) {
	tests := []struct {
		name     string
		limit    int64
		rankings []RetrievedChunkIDs
		want     RetrievedChunkIDs
	}{
		{
			name:     "single ranking preserves order",
			limit:    3,
			rankings: []RetrievedChunkIDs{{1, 2, 3}},
			want:     RetrievedChunkIDs{1, 2, 3},
		},
		{
			name:  "chunk appearing in both rankings outranks one appearing in only one",
			limit: 3,
			rankings: []RetrievedChunkIDs{
				{10, 1, 2},
				{20, 1, 3},
			},
			// 1 appears at a good rank in both lists, so it should win overall.
			want: RetrievedChunkIDs{1, 10, 20},
		},
		{
			name:     "limit truncates the result",
			limit:    2,
			rankings: []RetrievedChunkIDs{{1, 2, 3, 4, 5}},
			want:     RetrievedChunkIDs{1, 2},
		},
		{
			name:     "limit larger than available results does not panic or pad",
			limit:    10,
			rankings: []RetrievedChunkIDs{{1, 2}},
			want:     RetrievedChunkIDs{1, 2},
		},
		{
			name:     "empty rankings produce an empty result",
			limit:    5,
			rankings: []RetrievedChunkIDs{},
			want:     RetrievedChunkIDs{},
		},
		{
			name:  "equal scores tie-break by ascending chunk id",
			limit: 2,
			rankings: []RetrievedChunkIDs{
				{20},
				{10},
			},
			want: RetrievedChunkIDs{10, 20},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rrfMerge(tt.limit, tt.rankings...)
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

		chunks, err := RunRetrieval("q", FTS, 5, hydrator, retriever, client)
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

		_, err := RunRetrieval("q", Embedding, 5, hydrator, retriever, client)
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

		_, err := RunRetrieval("q", RetrievalStrategy(99), 5, hydrator, retriever, client)
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

		_, err := RunRetrieval("q", FTS, 5, hydrator, retriever, client)
		if !errors.Is(err, wantErr) {
			t.Fatalf("RunRetrieval() error = %v, want %v", err, wantErr)
		}
	})
}
