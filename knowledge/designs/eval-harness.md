---
type: design
title: Eval harness (RC1)
status: locked
date: 2026-10-17
tags: [eval, rc1, retrieval, metrics]
---

# Eval harness (RC1)

Implements [RC1](../requirements/rag-cli-handover.md): one quality command, metrics for each run, difference to the last run. Phase 1 of the [backlog](../requirements/backlog.md).

## Scope

- In: a CLI command `eval` that runs the golden set against the current DB for each strategy, gives metrics, writes a run file and shows the difference to the previous run.
- Out: golden set editor, LLM judge, answer quality. The harness measures retrieval only.

## Data outside the DB

- The golden set and the run files are files. They are not DB tables.
- Reason: the DB is a derived artifact ([0002](../decisions/0002-document-identity-derived-db.md)). A re-ingest deletes it. The eval history must stay.
- The WIP branch `golden_q` has eval tables in `sqlc_schema.sql`. Do not use them.
- Golden sets and run files contain corpus content. Git ignores `eval/golden*.json`. Run files go to `eval/runs/`, which git must also ignore.

## Golden set format

Keep the current format of `eval/golden.json`:

```json
{"queries": [{"id": "g01", "query": "...", "expect": [{"doc": "x.md", "breadcrumb_contains": "...", "text_contains": "..."}], "note": "...", "no_answer": false}]}
```

- `doc` matches `documents.name` now. After [0001](../decisions/0001-config-yaml-data-sources.md) and 0002 it becomes (collection, relative path). The loader must then accept the new form.
- An expectation matches a chunk when the `doc` is equal and each condition that is set matches (AND).
- A query is a hit when one expectation matches (OR).
- Substring match: casefold, and collapse all whitespace to one space on both sides.
- `text_contains` matches the index text (`chunks.text`). Reason: the render text changes with the renderer (backlog phase 5). The index text is stable.

## Status for each query and strategy

| Status | Meaning | What it shows |
|---|---|---|
| `hit` | A matching chunk is in the top k. The run file records its rank. | |
| `ranked_out` | No match in the top k, but a chunk of the doc matches. | A rank problem. |
| `content_lost` | The doc exists, but no chunk of the doc matches. | Pipeline content loss, or a wrong anchor. |
| `doc_missing` | No document with this name. | A stale golden set or a missing ingest. |

- `no_answer` queries have no status. The run file records the top 1 chunk and its score.
- The status check needs a port that reads all chunks of one doc. It does not need the retrieval code.

## Metrics for each strategy

- recall@1, @3, @5, @10 over the queries with expectations.
- MRR (reciprocal rank of the first hit, 0 for a miss).
- distinct-sections@10: mean count of distinct breadcrumbs in the top 10.
- Counts of `ranked_out`, `content_lost` and `doc_missing`.

## Run file

- One JSON file for each run: `eval/runs/<UTC timestamp>.json`.
- Header: timestamp, git commit (+ dirty flag), golden file path and `sha256`, DB path, chunk count, embed model and dim, k, strategies.
- Body: for each strategy the metrics, and for each query the status, the rank and the top k chunk IDs with scores.
- Reason for the header: a difference is only valid when the golden set and the model are the same. The command warns when the header differs from the previous run.

## Difference to the previous run

- The previous run is the newest file in the runs directory, or the file from `--baseline`.
- Show the metric deltas for each strategy.
- Show the status changes for each query, for example `g07 hybrid: hit@3 → ranked_out`. This is the most useful part of the output.

## Code layout

Follow the ports and adapters of [0000](../decisions/0000-migrated-baseline.md):

- `internal/eval/step`: `RunEval`, the matcher, the metrics, the diff. Pure logic, tests with fakes.
- Ports in `internal/eval/step/ports.go`:
  - `Searcher`: `Search(ctx, query, k, strategy) ([]RetrievedChunk, error)`. The adapter calls `retrieval/step.RunRetrieval`.
  - `DocChunks`: `ChunksOfDoc(ctx, docName) ([]Chunk, bool, error)`. The `bool` is false when the doc does not exist.
  - `GoldenSource`, `RunStore`: read the golden file, read and write run files.
- `cmd/cli/cmd/eval.go`: composition root. Move the retrieval wiring from `retrieve.go` into one shared function, so `retrieve`, `eval` and `serve` build the same deps.
- Flags: `--golden`, `--runs-dir`, `--baseline`, `--k` (default 10), `--strategies` (default `fts,ann,hybrid`), and the embed flags of `retrieve`. Later the golden path comes from `config.yaml`.
- Recipe `just eval`.
- Output: a table on stdout and the run file.

## Settled points

- k = 10. Reason: the MCP tool gives max 10 chunks to the model. The eval measures what the model can see.
- No rank in a larger pool for `ranked_out` now. Add it with RC7, when the reranker needs it.
- The current golden set (33 queries, 3 `no_answer`) is the first baseline.
