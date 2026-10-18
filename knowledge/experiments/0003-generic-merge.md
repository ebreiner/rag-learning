---
type: experiment
title: Merge generic chunks with neighbouring paragraphs (S0.2)
status: done
date: 2026-10-17
tags: [chunk, s0, generic, merge, cycle-1]
---

# 0003 Merge generic chunks with neighbouring paragraphs (S0.2)

Scope S0 of [unstable-scopes](../designs/unstable-scopes.md). Follows [0002](0002-generic-breadcrumb.md).
Hypothesis: a docling inline group is a paragraph that docling cut at bold or italic text. If it merges like a paragraph, the chunks become longer, the breadcrumb is a smaller part of each embedding, and fewer siblings compete for the top 10.

## Setup

- Branch `claude-unstable`, cycle 1. Base run `20261017T152200Z`. Previous run (S0.1) `20261017T162509Z`.
- Change: commit `e336bbd`. A group candidate goes into the content merge buffer with all its member nodes, under the same rules as a paragraph (same breadcrumb, budget 1000). The result type is `content`. The S0.1 helper is removed, because the normal flush adds the prefix.
- Full re-import, then `just eval`. Run `20261017T164359Z`. Same golden set and corpus.

## Result

Chunks: 1806 → 1116. Types now: `content` 788 (mean 520 chars), `list` 307, `table` 13, `code` 8. No `generic` chunks.

| Strategy | r@1 | r@3 | r@5 | r@10 | MRR | sect@10 | ranked_out |
|---|---|---|---|---|---|---|---|
| fts | 0.30 | 0.53 | 0.63 | 0.73 | 0.447 | 8.1 | 8 |
| ann | 0.40 | 0.60 | 0.67 | 0.87 | 0.533 | 8.7 | 4 |
| hybrid | 0.30 | 0.63 | 0.80 | 0.87 | 0.503 | 8.3 | 4 |

Against the base run (before S0):

| Strategy | Δr@1 | Δr@3 | Δr@5 | Δr@10 | ΔMRR | Δsect@10 |
|---|---|---|---|---|---|---|
| fts | +0.03 | +0.06 | +0.13 | +0.06 | +0.068 | +0.4 |
| ann | ±0 | +0.07 | +0.10 | +0.17 | +0.059 | +0.8 |
| hybrid | +0.03 | +0.16 | +0.20 | +0.10 | +0.095 | +0.4 |

## Findings

1. **S0.2 is the largest gain so far.** Hybrid r@5 goes from 0.60 to 0.80, MRR from 0.408 to 0.503. No strategy is worse than the base at any level.
2. **The S0.1 side effects are gone.** ANN r@1 is back at 0.40. distinct-sections@10 is higher than in the base in each strategy.
3. **`g04` and `g25` become hits at rank 1–2.** `g04` had no breadcrumb. Its fragments now merge into one chunk with enough text.
4. **The remaining misses are mostly "correct document, other chunk".** In `g06` (hybrid), rank 1 is a chunk of the expected document, but the anchor is in a different section of it. In `g19`, chunks of the expected document are at rank 4 and 9, but not the anchor chunk. The eval counts these as `ranked_out`. A document-level hit rate would show them; that is an eval change for the master backlog, not for this cycle.
5. **Hybrid is still below ANN at rank 1** (0.30 vs. 0.40), but better at r@3 and r@5. This is the input for the S1 fusion experiment.
6. **NaN chunks:** 2 chunks fail to embed (before: more, with smaller chunks). Not investigated.

## Recommendation

- Keep S0.2. It replaces the S0.1 mechanism and keeps its gain.
- Master backlog: the "generic chunks" row is solved by S0.1 + S0.2. Propose a decision at compaction: a docling inline group is a paragraph for the merge.
- Eval backlog for master: add a document-level hit rate (finding 4).
- Next: S1 fusion weights (cycle 1, experiment 3), to close the rank-1 gap between hybrid and ANN.
