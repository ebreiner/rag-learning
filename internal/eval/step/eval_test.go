package step

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var testCtx = context.Background()

type fakeDocs struct {
	chunks map[string][]Chunk // a doc name that is not a key does not exist
	err    error
}

func (f fakeDocs) ChunksOfDoc(ctx context.Context, docName string) ([]Chunk, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	c, ok := f.chunks[docName]
	return c, ok, nil
}

type fakeSearcher struct {
	hits   map[string][]SearchHit // key: strategy + "|" + query
	err    error
	calls  []string
	kCalls []int64
}

func (f *fakeSearcher) Search(ctx context.Context, query string, k int64, strategy string) ([]SearchHit, error) {
	f.calls = append(f.calls, strategy+"|"+query)
	f.kCalls = append(f.kCalls, k)
	if f.err != nil {
		return nil, f.err
	}
	return f.hits[strategy+"|"+query], nil
}

func hit(id int64, doc, breadcrumb, text string) SearchHit {
	return SearchHit{Chunk: Chunk{ID: id, DocName: doc, Breadcrumb: breadcrumb, Text: text}}
}

func TestClassify(t *testing.T) {
	docs := fakeDocs{chunks: map[string][]Chunk{
		"a.md":     {{ID: 1, DocName: "a.md", Breadcrumb: "Intro", Text: "the anchor text"}},
		"empty.md": {},
	}}
	other := hit(9, "x.md", "X", "noise")

	tests := []struct {
		name     string
		gq       GoldenQuery
		topK     []SearchHit
		want     Status
		wantRank int
		wantDoc  int // DocRank
	}{
		{
			name:     "hit at rank 2",
			gq:       GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md", TextContains: "anchor"}}},
			topK:     []SearchHit{other, hit(1, "a.md", "Intro", "the anchor text")},
			want:     StatusHit,
			wantRank: 2,
			wantDoc:  2,
		},
		{
			name:     "hit at rank 3, another chunk of the doc at rank 1",
			gq:       GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md", TextContains: "anchor"}}},
			topK:     []SearchHit{hit(2, "a.md", "Other", "no match"), other, hit(1, "a.md", "Intro", "the anchor text")},
			want:     StatusHit,
			wantRank: 3,
			wantDoc:  1,
		},
		{
			name: "ranked out: anchor in a chunk of the doc, not in top k",
			gq:   GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md", TextContains: "anchor"}}},
			topK: []SearchHit{other},
			want: StatusRankedOut,
		},
		{
			name:    "ranked out, but a different chunk of the doc is in the top k",
			gq:      GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md", TextContains: "anchor"}}},
			topK:    []SearchHit{other, hit(2, "a.md", "Other", "no match")},
			want:    StatusRankedOut,
			wantDoc: 2,
		},
		{
			name: "doc rank uses the doc of each expectation",
			gq: GoldenQuery{ID: "q", Expect: []Expectation{
				{Doc: "nope.md"},
				{Doc: "a.md", TextContains: "anchor"},
			}},
			topK:    []SearchHit{other, hit(2, "a.md", "Other", "no match")},
			want:    StatusRankedOut,
			wantDoc: 2,
		},
		{
			name: "content lost: doc exists, anchor in no chunk",
			gq:   GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md", TextContains: "missing words"}}},
			topK: []SearchHit{other},
			want: StatusContentLost,
		},
		{
			name: "content lost: doc exists with zero chunks",
			gq:   GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "empty.md"}}},
			want: StatusContentLost,
		},
		{
			name: "doc missing",
			gq:   GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "nope.md"}}},
			topK: []SearchHit{other},
			want: StatusDocMissing,
		},
		{
			name: "best status of all expectations wins",
			gq: GoldenQuery{ID: "q", Expect: []Expectation{
				{Doc: "nope.md"},
				{Doc: "a.md", TextContains: "anchor"},
			}},
			topK: []SearchHit{other},
			want: StatusRankedOut,
		},
		{
			name: "content lost is better than doc missing",
			gq: GoldenQuery{ID: "q", Expect: []Expectation{
				{Doc: "a.md", TextContains: "missing words"},
				{Doc: "nope.md"},
			}},
			want: StatusContentLost,
		},
		{
			name: "no_answer has no status",
			gq:   GoldenQuery{ID: "q", NoAnswer: true},
			topK: []SearchHit{other},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := classify(testCtx, tt.gq, tt.topK, 10, docs)
			if err != nil {
				t.Fatalf("classify() error = %v", err)
			}
			if got.Status != tt.want || got.Rank != tt.wantRank || got.DocRank != tt.wantDoc {
				t.Errorf("classify() = %s@%d doc@%d, want %s@%d doc@%d", got.Status, got.Rank, got.DocRank, tt.want, tt.wantRank, tt.wantDoc)
			}
		})
	}
}

// a matching chunk is always a chunk of an expected doc, so the doc rank is never after the hit rank
func TestDocRankNotAfterHitRank(t *testing.T) {
	gq := GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md", TextContains: "anchor"}, {Doc: "b.md"}}}
	pool := []SearchHit{
		hit(1, "x.md", "X", "noise"),
		hit(2, "a.md", "A", "no match"),
		hit(3, "b.md", "B", "any"),
		hit(4, "a.md", "A", "the anchor"),
	}
	// each rotation of the pool puts the hits at other ranks
	for shift := range pool {
		topK := append(append([]SearchHit{}, pool[shift:]...), pool[:shift]...)
		got, err := classify(testCtx, gq, topK, 10, fakeDocs{})
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != StatusHit || got.DocRank == 0 || got.DocRank > got.Rank {
			t.Errorf("shift %d: %s@%d doc@%d, want a hit with 0 < doc rank <= rank", shift, got.Status, got.Rank, got.DocRank)
		}
	}
}

func TestClassifyDocError(t *testing.T) {
	gq := GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md"}}}
	_, err := classify(testCtx, gq, nil, 10, fakeDocs{err: errors.New("db down")})
	if err == nil {
		t.Fatal("classify() error = nil, want error")
	}
}

func TestRunEval(t *testing.T) {
	gs := GoldenSet{Queries: []GoldenQuery{
		{ID: "g1", Query: "q1", Expect: []Expectation{{Doc: "a.md"}}},
		{ID: "g2", Query: "q2", NoAnswer: true},
	}}
	searcher := &fakeSearcher{hits: map[string][]SearchHit{
		"fts|q1": {hit(1, "a.md", "Intro", "text")},
	}}
	docs := fakeDocs{chunks: map[string][]Chunk{"a.md": {{ID: 1, DocName: "a.md"}}}}

	got, err := RunEval(testCtx, gs, []string{"fts", "ann"}, 10, 40, searcher, docs)
	if err != nil {
		t.Fatalf("RunEval() error = %v", err)
	}

	wantCalls := []string{"fts|q1", "fts|q2", "ann|q1", "ann|q2"}
	if diff := cmp.Diff(wantCalls, searcher.calls); diff != "" {
		t.Errorf("search calls (-want +got):\n%s", diff)
	}

	if len(got) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(got))
	}
	if got[0].Strategy != "fts" || got[0].Queries[0].Status != StatusHit {
		t.Errorf("fts g1 = %s %s, want fts hit", got[0].Strategy, got[0].Queries[0].Status)
	}
	if got[1].Strategy != "ann" || got[1].Queries[0].Status != StatusRankedOut {
		t.Errorf("ann g1 = %s %s, want ann ranked_out", got[1].Strategy, got[1].Queries[0].Status)
	}
	if got[0].Metrics.RecallAt[1] != 1 {
		t.Errorf("fts recall@1 = %v, want 1", got[0].Metrics.RecallAt[1])
	}
	for _, k := range searcher.kCalls {
		if k != 40 {
			t.Errorf("search k = %d, want 40 (the pool size, deeper than k)", k)
		}
	}
}

func TestRunEvalDepthIsAtLeastK(t *testing.T) {
	gs := GoldenSet{Queries: []GoldenQuery{{ID: "g1", Query: "q1", NoAnswer: true}}}
	searcher := &fakeSearcher{}

	if _, err := RunEval(testCtx, gs, []string{"fts"}, 10, 0, searcher, fakeDocs{}); err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]int64{10}, searcher.kCalls); diff != "" {
		t.Errorf("search k (-want +got):\n%s", diff)
	}
}

func TestClassifyInPool(t *testing.T) {
	gq := GoldenQuery{ID: "q", Expect: []Expectation{{Doc: "a.md", TextContains: "anchor"}}}
	docs := fakeDocs{chunks: map[string][]Chunk{"a.md": {{ID: 1, DocName: "a.md", Text: "the anchor"}}}}
	hits := []SearchHit{
		hit(7, "x.md", "X", "noise"),
		hit(8, "y.md", "Y", "noise"),
		hit(1, "a.md", "A", "the anchor"),
	}

	got, err := classify(testCtx, gq, hits, 2, docs)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusInPool || got.Rank != 3 {
		t.Errorf("classify() = %s@%d, want in_pool@3", got.Status, got.Rank)
	}
	if len(got.TopK) != 2 {
		t.Errorf("len(TopK) = %d, want 2: the run file keeps only the top k", len(got.TopK))
	}
	if got.DocRank != 0 {
		t.Errorf("DocRank = %d, want 0: the doc is not in the top k", got.DocRank)
	}

	got, err = classify(testCtx, gq, hits[:2], 2, docs)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusRankedOut {
		t.Errorf("classify() without the match in the hits = %s, want ranked_out", got.Status)
	}
}

func TestRunEvalSearchErrorStopsRun(t *testing.T) {
	gs := GoldenSet{Queries: []GoldenQuery{{ID: "g1", Query: "q1"}, {ID: "g2", Query: "q2"}}}
	searcher := &fakeSearcher{err: errors.New("embed down")}

	_, err := RunEval(testCtx, gs, []string{"ann"}, 10, 40, searcher, fakeDocs{})
	if err == nil {
		t.Fatal("RunEval() error = nil, want error")
	}
	if len(searcher.calls) != 1 {
		t.Errorf("search calls = %d, want 1", len(searcher.calls))
	}
}
