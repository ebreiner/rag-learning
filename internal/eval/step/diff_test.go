package step

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDiff(t *testing.T) {
	header := RunHeader{GoldenSHA256: "abc", EmbedModel: "bge-m3", EmbedDim: 1024, K: 10}
	prev := Run{Header: header, Strategies: []StrategyResult{{
		Strategy: "hybrid",
		Metrics:  Metrics{RecallAt: map[int]float64{1: 0.5}, MRR: 0.5, RankedOut: 1},
		Queries: []QueryResult{
			{QueryID: "g1", Status: StatusHit, Rank: 3},
			{QueryID: "g2", Status: StatusRankedOut},
			{QueryID: "g3", Status: StatusHit, Rank: 1},
		},
	}}}
	cur := Run{Header: header, Strategies: []StrategyResult{
		{
			Strategy: "hybrid",
			Metrics:  Metrics{RecallAt: map[int]float64{1: 0.75}, MRR: 0.25, RankedOut: 2},
			Queries: []QueryResult{
				{QueryID: "g1", Status: StatusRankedOut},
				{QueryID: "g2", Status: StatusHit, Rank: 1},
				{QueryID: "g3", Status: StatusHit, Rank: 1},
				{QueryID: "g4", Status: StatusHit, Rank: 1}, // new query, no change
			},
		},
		{Strategy: "fts"}, // not in prev
	}}

	deltas, changes, warnings := Diff(prev, cur)

	wantDeltas := map[string]Metrics{
		"hybrid": {RecallAt: map[int]float64{1: 0.75 - 0.5}, MRR: 0.25 - 0.5, RankedOut: 1},
	}
	if diff := cmp.Diff(wantDeltas, deltas); diff != "" {
		t.Errorf("deltas (-want +got):\n%s", diff)
	}

	wantChanges := []StatusChange{
		{QueryID: "g1", Strategy: "hybrid", From: "hit@3", To: "ranked_out"},
		{QueryID: "g2", Strategy: "hybrid", From: "ranked_out", To: "hit@1"},
	}
	if diff := cmp.Diff(wantChanges, changes); diff != "" {
		t.Errorf("changes (-want +got):\n%s", diff)
	}

	wantWarnings := []string{"strategy fts is not in the previous run"}
	if diff := cmp.Diff(wantWarnings, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}

func TestDiffHeaderWarnings(t *testing.T) {
	prev := Run{Header: RunHeader{GoldenSHA256: "abc", EmbedModel: "bge-m3", EmbedDim: 1024, K: 10}}
	cur := Run{Header: RunHeader{GoldenSHA256: "def", EmbedModel: "qwen", EmbedDim: 768, K: 5}}

	_, _, warnings := Diff(prev, cur)

	want := []string{
		"golden set changed since the previous run",
		"embed model changed: bge-m3/1024 -> qwen/768",
		"k changed: 10 -> 5",
	}
	if diff := cmp.Diff(want, warnings); diff != "" {
		t.Errorf("warnings (-want +got):\n%s", diff)
	}
}
