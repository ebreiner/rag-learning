package step

var recallLevels = []int{1, 3, 5, 10}

func computeMetrics(results []QueryResult, k int64) Metrics {
	m := Metrics{RecallAt: map[int]float64{}, DocRecallAt: map[int]float64{}}
	for _, at := range recallLevels {
		if int64(at) <= k {
			m.RecallAt[at] = 0
			m.DocRecallAt[at] = 0
		}
	}

	n, sections := 0, 0
	for _, r := range results {
		if r.Status == "" {
			continue
		}
		n++

		switch r.Status {
		case StatusHit:
			m.MRR += 1.0 / float64(r.Rank)
			for at := range m.RecallAt {
				if r.Rank <= at {
					m.RecallAt[at]++
				}
			}
		case StatusInPool:
			m.InPool++
		case StatusRankedOut:
			m.RankedOut++
		case StatusContentLost:
			m.ContentLost++
		case StatusDocMissing:
			m.DocMissing++
		}

		// for each status: a ranked_out query can still have its doc in the top k
		if r.DocRank > 0 {
			m.DocMRR += 1.0 / float64(r.DocRank)
			for at := range m.DocRecallAt {
				if r.DocRank <= at {
					m.DocRecallAt[at]++
				}
			}
		}

		// two docs can have the same breadcrumb, so the doc is part of the key
		seen := map[string]bool{}
		for _, h := range r.TopK {
			seen[h.DocName+"\x00"+h.Breadcrumb] = true
		}
		sections += len(seen)
	}

	if n == 0 {
		return m
	}
	for at := range m.RecallAt {
		m.RecallAt[at] /= float64(n)
	}
	m.MRR /= float64(n)
	for at := range m.DocRecallAt {
		m.DocRecallAt[at] /= float64(n)
	}
	m.DocMRR /= float64(n)
	m.DistinctSect = float64(sections) / float64(n)

	return m
}
