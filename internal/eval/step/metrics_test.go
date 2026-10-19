package step

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestComputeMetrics(t *testing.T) {
	results := []QueryResult{
		{Status: StatusHit, Rank: 1, DocRank: 1, TopK: []SearchHit{
			hit(1, "a.md", "A", ""), hit(2, "a.md", "A", ""), hit(3, "b.md", "A", ""),
		}},
		{Status: StatusHit, Rank: 4, DocRank: 2, TopK: []SearchHit{hit(4, "a.md", "B", "")}},
		{Status: StatusRankedOut, DocRank: 3}, // doc in the top k, but not the expected chunk
		{Status: StatusContentLost},
		{Status: ""}, // no_answe, not counted
	}

	got := computeMetrics(results, 10)

	n := 4.0
	want := Metrics{
		RecallAt: map[int]float64{
			1:  1 / n,
			3:  1 / n,
			5:  2 / n,
			10: 2 / n,
		},
		MRR: (1.0 + 1.0/4.0) / n,
		DocRecallAt: map[int]float64{
			1:  1 / n,
			3:  3 / n,
			5:  3 / n,
			10: 3 / n,
		},
		DocMRR:       (1.0 + 1.0/2.0 + 1.0/3.0) / n,
		DistinctSect: (2.0 + 1.0) / n, // a.md>A and b.md>A are distinct; a.md>B
		RankedOut:    1,
		ContentLost:  1,
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("computeMetrics() (-want +got):\n%s", diff)
	}
}

func TestComputeMetricsLevelsAboveK(t *testing.T) {
	got := computeMetrics([]QueryResult{{Status: StatusHit, Rank: 1}}, 3)

	want := map[int]float64{1: 1, 3: 1}
	if diff := cmp.Diff(want, got.RecallAt); diff != "" {
		t.Errorf("RecallAt (-want +got):\n%s", diff)
	}
}

func TestComputeMetricsNoQueries(t *testing.T) {
	got := computeMetrics([]QueryResult{{Status: ""}}, 10)

	if got.MRR != 0 || got.DistinctSect != 0 || got.RecallAt[1] != 0 {
		t.Errorf("computeMetrics() = %+v, want zero values", got)
	}
}

// in_pool is a miss for the top k: it counts in InPool, not in recall or MRR
func TestComputeMetricsInPool(t *testing.T) {
	results := []QueryResult{
		{Status: StatusHit, Rank: 1},
		{Status: StatusInPool, Rank: 15},
	}

	got := computeMetrics(results, 10)

	if got.InPool != 1 || got.RankedOut != 0 {
		t.Errorf("InPool = %d, RankedOut = %d, want 1 and 0", got.InPool, got.RankedOut)
	}
	if got.MRR != 0.5 || got.RecallAt[10] != 0.5 {
		t.Errorf("MRR = %v, r@10 = %v, want 0.5 and 0.5", got.MRR, got.RecallAt[10])
	}
}
