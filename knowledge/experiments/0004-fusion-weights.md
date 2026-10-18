---
type: experiment
title: Weighted RRF fusion for the hybrid strategy (S1)
status: done
date: 2026-10-18
tags: [retrieval, s1, fusion, rrf, cycle-1]
---

# 0004 Weighted RRF fusion for the hybrid strategy (S1)

Scope S1 of [unstable-scopes](../designs/unstable-scopes.md). Follows [0003](0003-generic-merge.md).
Hypothesis: after S0.2, ANN is better than hybrid at rank 1 (r@1 0.40 vs. 0.30, MRR 0.533 vs. 0.503). A higher ANN weight in the RRF, or a smaller RRF k, moves hybrid nearer to ANN at rank 1 and keeps the hybrid gain at r@3 and r@5.

## Setup

- Branch `claude-unstable`, cycle 1. Base run `20261017T152200Z`. Reference run (S0.2) `20261017T164359Z`.
- Change: commit `3765337`. `FusionConfig` in `RetrievalDeps` (`RRFK`, `WeightFTS`, `WeightANN`). The zero value gives the old behaviour (k 60, weights 1/1). Each RRF term is `weight / (k + rank)`. The composition root reads `RAG_FUSION_RRF_K`, `RAG_FUSION_WEIGHT_FTS` and `RAG_FUSION_WEIGHT_ANN`.
- No re-import: the change is after the index. The DB holds the S0.2 chunks (1116). Only the env changes between the runs. FTS and ANN results are the same in all runs.
- `WeightFTS` stays 1.0 in all runs. Same golden set (`golden_sha256` `455c6f4e…`) and corpus.
- The runs of 2026-10-18 show `git_dirty: true`. The only dirty file is the `justfile` (OTLP endpoint). It has no effect on retrieval. Commit `b7fab68` between the runs changes only the docling client.

## Result

Hybrid only. 30 golden queries, so one query is 0.033 recall.

| Run | ANN weight | RRF k | r@1 | r@3 | r@5 | r@10 | MRR | sect@10 | ranked_out |
|---|---|---|---|---|---|---|---|---|---|
| `20261017T164805Z` | 1.0 (control) | 60 | 0.30 | 0.63 | **0.80** | 0.87 | 0.503 | 8.3 | 4 |
| `20261017T164813Z` | 1.5 | 60 | 0.30 | **0.70** | 0.77 | 0.87 | 0.511 | 8.1 | 4 |
| `20261018T060803Z` | 2.0 | 60 | 0.30 | **0.70** | 0.73 | 0.87 | 0.505 | 8.1 | 4 |
| `20261018T060812Z` | 3.0 | 60 | **0.33** | **0.70** | 0.73 | **0.90** | **0.514** | 8.3 | 3 |
| `20261018T060822Z` | 2.0 | 20 | 0.30 | **0.70** | 0.73 | **0.90** | 0.503 | 8.2 | 3 |
| reference: ANN only | – | – | 0.40 | 0.60 | 0.67 | 0.87 | 0.533 | 8.7 | 4 |

The control run is identical to the S0.2 run. Thus the refactor to `weightedRRFMerge` does not change the old behaviour.

## Findings

1. **No grid point is a clear gain.** Each difference to the control is 1 or 2 queries. With 30 queries, this is in the range of noise.
2. **A higher ANN weight moves recall from r@5 to r@3.** r@3 goes from 0.63 to 0.70, but r@5 goes from 0.80 to 0.73. MRR changes by max. +0.011.
3. **Hybrid does not reach ANN at rank 1 in any grid point.** The best hybrid r@1 is 0.33 (ANN 3.0), ANN alone has 0.40. Also the best hybrid MRR (0.514) is below ANN alone (0.533).
4. **The rank-1 errors come from the RRF consensus.** In the control run, ANN has 4 queries at rank 1 that hybrid does not have (`g13`, `g18`, `g21`, `g23`). In each, the correct chunk is ANN #1, but low or absent in FTS. The hybrid rank 1 is a chunk that is high in both lists (FTS #1–3 and ANN #2–5). RRF prefers such a chunk by design. Only a strong ANN weight changes this, but then hybrid becomes ANN and loses the FTS gain at r@5.
5. **Some queries are unstable at the top 10.** `g17` and `g23` change between hit and `ranked_out` from one grid point to the next. They are near rank 10 in all configurations. `g17` is a golden-set candidate from 0001.
6. **The `no_answer` queries** (`g31`–`g33`) have no clear score gap in any strategy. Fusion weights cannot make a "no answer" threshold.

## Recommendation

- Keep the `FusionConfig` port and the zero-value default (k 60, weights 1/1). Do not change the default weights. The data does not support a different value.
- Do not tune fusion weights more with the current golden set. 30 queries are too few to see differences of 1–2 queries.
- The rank-1 gap needs a stage after the fusion: the reranker of scope S1 (cycle 4 in [unstable-scopes](../designs/unstable-scopes.md)). A cross-encoder scores query and chunk together, so it can find the correct chunk also when only one list ranks it high (finding 4).
- Eval backlog for master: more golden queries, so that a gain of 0.03 becomes measurable. Also the document-level hit rate from [0003](0003-generic-merge.md).
