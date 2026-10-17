package step

import (
	"context"
	"fmt"
)

func RunEval(ctx context.Context, gs GoldenSet, strategies []string, k int64, searcher Searcher, docs DocChunksProvider) ([]StrategyResult, error) {
	results := make([]StrategyResult, 0, len(strategies))
	for _, strategy := range strategies {
		queryResults := make([]QueryResult, 0, len(gs.Queries))
		for _, gq := range gs.Queries {
			topK, err := searcher.Search(ctx, gq.Query, k, strategy)
			if err != nil {
				return nil, fmt.Errorf("search %s with strategy %s: %w", gq.ID, strategy, err)
			}

			res, err := classify(ctx, gq, topK, docs)
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

func classify(ctx context.Context, gq GoldenQuery, topK []SearchHit, docs DocChunksProvider) (QueryResult, error) {
	res := QueryResult{QueryID: gq.ID, TopK: topK}
	if gq.NoAnswer {
		return res, nil
	}

	for i, h := range topK {
		for _, e := range gq.Expect {
			if matches(e, h.Chunk) {
				res.Status, res.Rank = StatusHit, i+1
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
