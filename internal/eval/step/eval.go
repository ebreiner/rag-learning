package step

import (
	"context"
	"fmt"
)

func RunEval(ctx context.Context, gs GoldenSet, strategies []string, k, poolSize int64, searcher Searcher, docs DocChunksProvider) ([]StrategyResult, error) {
	depth := max(k, poolSize)
	results := make([]StrategyResult, 0, len(strategies))
	for _, strategy := range strategies {
		queryResults := make([]QueryResult, 0, len(gs.Queries))
		for _, gq := range gs.Queries {
			hits, err := searcher.Search(ctx, gq.Query, depth, strategy)
			if err != nil {
				return nil, fmt.Errorf("search %s with strategy %s: %w", gq.ID, strategy, err)
			}

			res, err := classify(ctx, gq, hits, k, docs)
			if err != nil {
				return nil, fmt.Errorf("classify %s with strategy %s: %w", gq.ID, strategy, err)
			}
			queryResults = append(queryResults, res)
		}

		results = append(results, StrategyResult{
			Strategy: strategy,
			Metrics:  computeMetrics(queryResults, k),
			Queries:  queryResults,
		})
	}

	return results, nil
}

func classify(ctx context.Context, gq GoldenQuery, hits []SearchHit, k int64, docs DocChunksProvider) (QueryResult, error) {
	topK := hits
	if len(topK) > int(k) {
		topK = hits[:k]
	}
	// the run file keeps only the top k, the hits below k are only for in_pool
	res := QueryResult{QueryID: gq.ID, TopK: topK}
	if gq.NoAnswer {
		return res, nil
	}
	res.DocRank = docRank(gq, topK)

	for i, h := range hits {
		for _, e := range gq.Expect {
			if matches(e, h.Chunk) {
				res.Status, res.Rank = StatusHit, i+1
				if i >= len(topK) {
					res.Status = StatusInPool
				}
				return res, nil
			}
		}
	}

	best := StatusDocMissing
	for _, e := range gq.Expect {
		chunks, exists, err := docs.ChunksOfDoc(ctx, e.Doc)
		if err != nil {
			return res, err
		}
		if !exists {
			continue
		}
		best = StatusContentLost
		for _, c := range chunks {
			if matches(e, c) {
				res.Status = StatusRankedOut
				return res, nil
			}
		}
	}

	res.Status = best
	return res, nil
}

func docRank(gq GoldenQuery, topK []SearchHit) int {
	expected := map[string]bool{}
	for _, e := range gq.Expect {
		expected[e.Doc] = true
	}
	for i, h := range topK {
		if expected[h.DocName] {
			return i + 1
		}
	}
	return 0
}
