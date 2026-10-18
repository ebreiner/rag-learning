---
type: experiment
title: Breadcrumb prefix for generic chunks (S0.1)
status: done
date: 2026-10-17
tags: [chunk, s0, generic, breadcrumb, cycle-1]
---

# 0002 Breadcrumb prefix for generic chunks (S0.1)

Scope S0 of [unstable-scopes](../designs/unstable-scopes.md). Hypothesis from [0001](0001-eval-baseline.md): `generic` chunks (66 % of all chunks) miss because their index text has no section signal.

## Setup

- Branch `claude-unstable`, cycle 1. Base: `unstable/1/base` (`f0514ab`), base run `20261017T152200Z` (same numbers as 0001).
- Change: commit `653f393`. A `generic` chunk gets `breadcrumb + "\n\n" + text` as index text, like a `content` chunk. An empty breadcrumb adds nothing. No other chunk type changes.
- Full re-import (`just e2e-clear`), then `just eval`. Run `20261017T162509Z`. Same golden set, same corpus, 1806 chunks.

## Result

| Strategy | r@1 | r@3 | r@5 | r@10 | MRR | sect@10 | ranked_out |
|---|---|---|---|---|---|---|---|
| fts | 0.30 (+0.03) | 0.50 (+0.03) | 0.53 (+0.03) | 0.70 (+0.03) | 0.426 (+0.047) | 7.2 (−0.5) | 9 (−1) |
| ann | 0.33 (−0.07) | 0.50 (−0.03) | 0.57 (±0) | 0.77 (+0.07) | 0.448 (−0.026) | 7.3 (−0.6) | 7 (−2) |
| hybrid | 0.27 (±0) | 0.60 (+0.13) | 0.70 (+0.10) | 0.80 (+0.03) | 0.436 (+0.028) | 7.2 (−0.7) | 6 (−1) |

Hybrid is the strategy of the MCP tool. It gains in r@3, r@5, r@10 and MRR, and loses nothing.

## Findings

1. **The section signal helps recall.** `g09` was a miss in each strategy. Now it is a hit in each strategy. FTS and hybrid improve at all levels.
2. **ANN loses at rank 1** (0.40 → 0.33). In `g07`, a chunk of a different document now wins rank 1, because its breadcrumb is very similar to the query. In `g06`, a short release-note chunk wins. In a short chunk, the breadcrumb is a large part of the text, so it dominates the embedding.
3. **Diversity goes down.** distinct-sections@10 drops by 0.5–0.7 in each strategy. Sibling chunks of one section now share the same prefix, so they look more similar and fill more slots of the top 10.
4. **Still full misses in hybrid:** `g04`, `g17`, `g19`, `g24` (plus `g23`, `g25`). `g04` has no breadcrumb, so S0.1 cannot help it. `g17` and `g24` are golden-set candidates from 0001.

## Recommendation

- Keep S0.1. It is a net gain for hybrid.
- Next: S0.2, merge `generic` chunks with neighbouring paragraphs under the same breadcrumb. Fewer and longer chunks for each section reduce findings 2 and 3: the breadcrumb is a smaller part of each chunk, and fewer siblings compete for the top 10.
- Watch ANN r@1 and sect@10 in S0.2. If the ANN rank-1 loss stays, the S1 fusion experiment must consider it.
