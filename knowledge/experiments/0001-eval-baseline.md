---
type: experiment
title: Eval baseline after full re-import
status: done
date: 2026-10-17
tags: [eval, rc1, baseline, chunk, retrieval]
---

# 0001 Eval baseline after full re-import

First valid baseline of the [eval harness](../designs/eval-harness.md). Later experiments compare with these numbers.

## Setup

- Code: commit `6f840cc` (branch `eval`). Full re-import from scratch: extract, chunk, embed.
- Corpus: local test corpus, 1806 chunks. Embed model `bge-m3`, dim 1024. k = 10.
- Golden set: 33 queries (30 with expectations, 3 `no_answer`), `sha256` `455c6f4e…`.
- Run file: `eval/runs/20261017T145021Z.json` (local, not in git).

## Result

| Strategy | r@1 | r@3 | r@5 | r@10 | MRR | ranked_out | content_lost | doc_missing |
|---|---|---|---|---|---|---|---|---|
| fts | 0.27 | 0.47 | 0.50 | 0.67 | 0.379 | 10 | 0 | 0 |
| ann | 0.40 | 0.53 | 0.57 | 0.70 | 0.474 | 9 | 0 | 0 |
| hybrid | 0.27 | 0.47 | 0.60 | 0.77 | 0.408 | 7 | 0 | 0 |

Run before the re-import (DB older than fix `812baa3`): `content_lost` = 1 (`g07`) in each strategy, hybrid r@10 = 0.73. The re-import added 150 chunks and fixed `g07`.

## Findings

1. **A stale DB is not visible in the run header.** The chunks were older than the chunk code. Fix: the header now records `chunks_built` (newest `chunks.created_at`).
2. **`generic` chunks have no section signal.** 1197 of 1806 chunks (66 %) are `generic` (docling inline groups: paragraphs with bold text or links). They get no breadcrumb prefix in `chunks.text` and do not merge with neighbours. All 5 queries that miss in each strategy (`g04`, `g09`, `g17`, `g19`, `g24`) expect a `generic` chunk.
3. **Hybrid is worse than ANN at rank 1** (0.27 vs. 0.40), but best at r@10 (0.77). In `g13` and `g21`, ANN has the hit at rank 1 and hybrid at rank 2. The MCP tool always uses hybrid.
4. **Some misses are golden-set problems.** `g09`: the top 1 is the correct section, another chunk. `g17`: the query says "Grenzwert", the doc says "Schwellwert". `g24`: the top 1 heading is equal to the query text.

## Recommendation

- Next experiment: breadcrumb prefix for `generic` chunks. Then, as a separate run, the merge of `generic` chunks.
- Review the golden set for findings 4 in a separate run.
- Evaluate finding 3 again after the `generic` change. If it stays, it supports the reranker (RC7).
