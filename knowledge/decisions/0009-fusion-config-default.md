---
type: decision
title: Fusion config port with equal weights as the default
status: accepted
date: 2026-10-18
tags: [retrieval, ranking, rrf, fusion, cycle-1]
---

# 0009 Fusion config port with equal weights as the default

Adds to [0000](0000-migrated-baseline.md): retrieval and ranking (`rrfMerge`, `rrfK = 60`). The default behaviour does not change.

## Context

- After [0008](0008-inline-group-merges-as-paragraph.md), ANN is better than hybrid at rank 1 (r@1 0.40 vs. 0.30, MRR 0.533 vs. 0.503).
- Hypothesis: a higher ANN weight or a smaller RRF k in the fusion closes this gap.
- Evidence: [experiment 0004](../experiments/0004-fusion-weights.md), commit `3765337` on `claude-unstable` (tag `unstable/1/end`). Five grid points: ANN weight 1.0, 1.5, 2.0 and 3.0 at k 60, and 2.0 at k 20.

## Options considered

1. Keep the fixed `rrfMerge`. Simple, but each later fusion test needs a code change.
2. A `FusionConfig` port with the old values as the default. Accepted.
3. Option 2 with a higher ANN weight as the default. No grid point beats equal weights: each difference is 1–2 of 30 queries. A higher ANN weight moves hits from r@5 to r@3, but adds no hits.

## Decision

- `RetrievalDeps` gets `FusionConfig` (`RRFK`, `WeightFTS`, `WeightANN`). Each RRF term is `weight / (k + rank)`.
- The zero value gives the default: k 60, weights 1/1. This is the old behaviour.
- The composition root can read `RAG_FUSION_RRF_K`, `RAG_FUSION_WEIGHT_FTS` and `RAG_FUSION_WEIGHT_ANN`. These env variables are a test tool for eval runs, not a documented user feature.
- No more tuning of the weights with the current golden set.

## Consequences

- The rank-1 gap stays. In the 4 queries that ANN has at rank 1 and hybrid does not, the hybrid rank 1 is a chunk that is high in both lists. RRF prefers such a chunk by design. The reranker of [0003](0003-remote-reranker.md) is the next step for this gap.
- With the default, `master` gives the same numbers as before. The user writes the port on `master` again.
- 30 golden queries are too few to measure a gain of 1–2 queries. The backlog gets more golden queries.
