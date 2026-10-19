---
type: decision
title: Eval status in_pool, search depth from the pool size
status: accepted
date: 2026-10-19
tags: [eval, rc1, retrieval, pool]
---

# 0014 Eval status in_pool, search depth from the pool size

Adds to [0007](0007-eval-harness-implementation.md) and the locked design [eval-harness](../designs/eval-harness.md). Changes the meaning of the status `ranked_out` from those documents.

## Context

- [0013](0013-retrieval-two-phases.md) splits retrieval into phase 1 (find a pool of candidates) and phase 2 (order the pool).
- The status `ranked_out` said only that the correct chunk is not in the top k. It did not show which phase failed. A miss in phase 1 needs a fix of chunks, FTS or embeddings. A miss in phase 2 needs a fix of the ranking, for example a reranker.

## Options considered

1. A one-off eval run with `k: 40` and a manual compare with the k=10 run. No code, but no number in each run.
2. A new status in the eval. Accepted.

## Decision

- The eval searches each query to the depth `max(k, retrieval.pool_size)`.
- Status `in_pool`: the correct chunk is at rank k+1 to the depth. The run file stores its rank.
- Status `ranked_out` now means: the chunk exists in a document, but it is not in the depth.
- r@n, MRR, doc metrics and `distinct_sections_at_10` use only the top k, as before.
- The run file keeps only the top k hits for each query.
- The run header has `pool_size`. The diff warns when the pool size of two runs is different.

## Consequences

- The existing metrics do not change. Run `20261019T135803Z` has the same r@n and MRR as the run before the change.
- A comparison with a run from before this change shows `ranked_out → in_pool` changes and the pool size warning.
- `20261019T135803Z` (pool 40) is the new reference run.
- First result for hybrid: 32 of 33 answer queries have the correct chunk in the pool, 22 of them not at rank 1, 10 at rank 2. Only one query is outside the pool. This makes phase 2 (RC7 reranker) the larger potential.
