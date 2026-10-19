package step

import "fmt"

// Diff compares two runs. The deltas are cur minus prev for each strategy in both runs.
// The warnings tell when the runs are not comparable.
func Diff(prev, cur Run) (deltas map[string]Metrics, changes []StatusChange, warnings []string) {
	ph, ch := prev.Header, cur.Header
	if ph.GoldenSHA256 != ch.GoldenSHA256 {
		warnings = append(warnings, "golden set changed since the previous run")
	}
	if ph.EmbedModel != ch.EmbedModel || ph.EmbedDim != ch.EmbedDim {
		warnings = append(warnings, fmt.Sprintf("embed model changed: %s/%d -> %s/%d", ph.EmbedModel, ph.EmbedDim, ch.EmbedModel, ch.EmbedDim))
	}
	if ph.K != ch.K {
		warnings = append(warnings, fmt.Sprintf("k changed: %d -> %d", ph.K, ch.K))
	}
	if ph.PoolSize != ch.PoolSize {
		warnings = append(warnings, fmt.Sprintf("pool size changed: %d -> %d, in_pool and ranked_out are not comparable", ph.PoolSize, ch.PoolSize))
	}

	prevByStrategy := map[string]StrategyResult{}
	for _, s := range prev.Strategies {
		prevByStrategy[s.Strategy] = s
	}

	deltas = map[string]Metrics{}
	for _, cs := range cur.Strategies {
		ps, ok := prevByStrategy[cs.Strategy]
		if !ok {
			warnings = append(warnings, fmt.Sprintf("strategy %s is not in the previous run", cs.Strategy))
			continue
		}
		deltas[cs.Strategy] = subtractMetrics(cs.Metrics, ps.Metrics)

		prevByQuery := map[string]QueryResult{}
		for _, q := range ps.Queries {
			prevByQuery[q.QueryID] = q
		}
		for _, cq := range cs.Queries {
			pq, ok := prevByQuery[cq.QueryID]
			if !ok {
				continue
			}
			from, to := statusLabel(pq), statusLabel(cq)
			if from != to {
				changes = append(changes, StatusChange{QueryID: cq.QueryID, Strategy: cs.Strategy, From: from, To: to})
			}
		}
	}

	return deltas, changes, warnings
}

func statusLabel(q QueryResult) string {
	if q.Status == StatusHit || q.Status == StatusInPool {
		return fmt.Sprintf("%s@%d", q.Status, q.Rank)
	}
	return string(q.Status)
}

func subtractMetrics(cur, prev Metrics) Metrics {
	d := Metrics{
		RecallAt:     map[int]float64{},
		DocRecallAt:  map[int]float64{},
		MRR:          cur.MRR - prev.MRR,
		DocMRR:       cur.DocMRR - prev.DocMRR,
		DistinctSect: cur.DistinctSect - prev.DistinctSect,
		InPool:       cur.InPool - prev.InPool,
		RankedOut:    cur.RankedOut - prev.RankedOut,
		ContentLost:  cur.ContentLost - prev.ContentLost,
		DocMissing:   cur.DocMissing - prev.DocMissing,
	}
	for at, v := range cur.RecallAt {
		d.RecallAt[at] = v - prev.RecallAt[at]
	}
	// a run file from before the doc metrics has no DocRecallAt: nil map reads give 0
	for at, v := range cur.DocRecallAt {
		d.DocRecallAt[at] = v - prev.DocRecallAt[at]
	}
	return d
}
