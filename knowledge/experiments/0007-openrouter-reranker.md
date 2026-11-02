---
type: experiment
title: Remote reranker through OpenRouter, source weight as a multiplier
status: open
date: 2026-10-25
tags: [retrieval, ranking, reranker, openrouter, eval, hybrid]
---

# 0007 Remote reranker through OpenRouter, source weight as a multiplier

Implements the stage of [0003](../decisions/0003-remote-reranker.md) for the backlog row RC7 in [backlog](../requirements/backlog.md).
The two-phase retrieval is in [0013](../decisions/0013-retrieval-two-phases.md).

## Setup

- Branch `claude_experimental`, uncommitted. The code:
  - Port `step.Reranker` in `internal/retrieval/step/ports.go`: one score for each text, in the input order. Optional field `RetrievalDeps.Reranker`.
  - `RunRetrieval`, hybrid phase 2: hydrate the pool, rerank the pool, multiply by the source weight (`sourceRerank`), cut to `k`.
    The reranker score replaces the RRF score. FTS and ANN do not use the reranker.
  - On an error, a timeout or a wrong score count, a warning goes into the log and the RRF score stays. The query does not fail.
  - OTel span `rerank` with `rerank.pool_size`, `rerank.model` and `rerank.cost`.
    The request sends the trace ID and the span ID of `rerank` in the `trace` field, so that OpenRouter Broadcast joins the trace.
  - Client `internal/platform/rerankclient/` (OpenRouter/Cohere format, `POST <url>` with `model`, `query`, `documents`, `trace`).
    The auth header comes from the `httpclient` header transport, so the dump and OTel do not see the key.
  - Config block `retrieval.rerank`: `enabled`, `url` (default `https://openrouter.ai/api/v1/rerank`), `model`, `api_key`, `timeout`.
    A missing block or `enabled: false` gives the old behavior. With `enabled: true`, `model`, `api_key` and `timeout > 0` are required.
- Rerank input text: the chunk text, the same text as the embed input at ingest (`internal/platform/sqlite/embedding/chunk_source.go` sends `chunks.text`).
  The breadcrumb is not added. In `data/data.db` (7700 chunks), 4321 of 7633 chunks with a breadcrumb start with it in the text.
  The other chunks have the breadcrumb only in the column.
- Plan: `just --set config <copy with the rerank block> eval 20261022T192853Z`, once with `cohere/rerank-4-fast`, once with `voyageai/rerank-2.5`.
  The eval uses `WireUpRetrieval`, so it uses the reranker of the config. `k` 10, `pool_size` 40.
- Baseline `20261022T192853Z` (no reranker), hybrid: chunk r@1 0.36, r@3 0.58, r@5 0.67, r@10 0.70, MRR 0.489;
  doc r@1 0.70, r@3 0.73, r@5 0.88, r@10 0.88, doc MRR 0.739.

## Result

The first runs use the DB after the table change ([0008](0008-table-rows.md)), so the baseline is `20261025T094828Z` (same chunks, no reranker).
The OpenRouter guardrail requires zero data retention (ZDR). `cohere/rerank-4-fast` has no ZDR endpoint and fails with 404. Of the rerank models, only `qwen/qwen3-reranker-8b` (Fireworks) has ZDR.

| model | run | rerank calls failed | chunk r@1 | chunk r@10 | chunk MRR | doc r@1 | doc r@3 | doc r@5 | doc r@10 | doc MRR |
|---|---|---|---|---|---|---|---|---|---|---|
| none (baseline) | `20261025T094828Z` | – | 0.36 | 0.73 | 0.494 | 0.67 | 0.73 | 0.85 | 0.88 | 0.723 |
| `qwen/qwen3-reranker-8b` | `20261025T121333Z` | 36 of 36 (model not in the guardrail) | not valid | | | | | | | |
| `qwen/qwen3-reranker-8b` | `20261025T121651Z` | 10 of 36 (503 overloaded) | 0.39 | 0.76 | 0.535 | 0.70 | 0.79 | 0.91 | 0.94 | 0.769 |
| `qwen/qwen3-reranker-8b` | `20261025T121831Z` | 6 of 36 (503 overloaded) | 0.55 | 0.79 | 0.642 | 0.61 | 0.79 | 0.85 | 0.88 | 0.705 |
| `qwen/qwen3-reranker-8b` | `20261025T130010Z` | 4 of 36 (503 overloaded) | 0.58 | 0.82 | 0.671 | 0.70 | 0.85 | 0.91 | 0.94 | 0.786 |
| `qwen/qwen3-reranker-8b` | `20261025T133611Z` | 4 of 36 (503 overloaded) | 0.52 | 0.82 | 0.631 | 0.64 | 0.82 | 0.91 | 0.94 | 0.746 |
| `cohere/rerank-4-fast` | `20261025T134659Z` | 0 of 36 (ZDR enforcement off) | 0.55 | 0.94 | 0.687 | 0.73 | 0.88 | 0.91 | 0.97 | 0.806 |
| `voyageai/rerank-2.5` | `20261025T134723Z` | 0 of 36 (ZDR enforcement off) | 0.55 | 0.94 | 0.677 | 0.73 | 0.85 | 0.85 | 0.94 | 0.790 |

- A failed call keeps the RRF order for that query. Thus the two Qwen runs mix reranked and RRF queries, and the failed queries are not the same in both runs.
- Run `20261025T121831Z`, hybrid, status changes: 8 queries move to rank 1 (g05, g07, g09, g15, g16, g23, g26, g34); g06 and g20 move from the pool into the top 10. g22 drops out of the top 10 (7 → pool 12), g03 drops 2 → 6.
- Latency and cost per call, from the `rerank` spans (pool of 40 chunks):

  | model | calls | p50 ms | p95 ms | cost / call |
  |---|---|---|---|---|
  | `cohere/rerank-4-fast` | 36 | 430 | 485 | $0.0020 |
  | `voyageai/rerank-2.5` | 36 | 395 | 485 | $0.00031 |
  | `qwen/qwen3-reranker-8b` (Fireworks) | 123 ok, plus failed calls | 914 | 2041 | $0.0019 |

- Cohere and Voyage, hybrid: both lift chunk r@10 from 0.73 to 0.94 (pool hits in the top 10: 8 → 1). Voyage moves g19 from pool rank 37 into the top 10 (rank 7); Cohere leaves it in the pool (13).

## Findings

- The run header does not record the reranker or its model. Name the model with the run ID in this file.
- The source weight multiplies a reranker score in [0, 1] now, not an RRF score. The relative effect of a weight is the same, but the scale of `Score` changes.
- The reranker does not see the breadcrumb of about 43 % of the chunks with a breadcrumb. A run with breadcrumb + text can show if this has an effect.

## Recommendation

- Repeat the Qwen run until no rerank call fails (or add a retry), then compare. A mixed run is not evidence.
- Run Cohere and Voyage only if the guardrail permits non-ZDR endpoints for them.
- Fill in the latency and cost from the `rerank` spans.
- Record a decision on model, pool size and the order of weight and reranker only after the user agrees on the numbers.
