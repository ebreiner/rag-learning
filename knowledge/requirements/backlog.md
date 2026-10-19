---
type: requirement
title: Open backlog
status: open
date: 2026-10-19
tags: [backlog, handover, eval, data-source, retrieval, mcp, auth]
---

# Open backlog

- Open work only. The phase number is the priority. A lower number comes first.
- IDs `RCn` are from the [handover](../requirements/rag-cli-handover.md). The interpretation is in [rag-cli-handover-interpretation](rag-cli-handover-interpretation.md).
- "Old Tn" is the tier number of the backlog before 2026-10-17 (see [0000](../decisions/0000-migrated-baseline.md) and git history).

## Phases in priority order

| Phase | Need | Reason | Found | Status |
|---|---|---|---|---|
| 1 | **RC1 eval harness.** Done: `eval` command, `just eval`, run files, diff. See [eval-harness](../designs/eval-harness.md) and [0007](../decisions/0007-eval-harness-implementation.md). Baseline: [experiment 0001](../experiments/0001-eval-baseline.md). | RC7, RC13 and RC15 use RC1 as their acceptance test. No rank change without numbers. | Handover, design. | Done. The golden path, `k` and the strategies come from `eval:` in the config file ([0011](../decisions/0011-config-file-and-composition-root.md)). `--baseline` stays a flag. |
| 1 | Choose the eval corpus: the current test corpus, or a second Markdown test corpus. Add the real German PO questions when they come. | The golden set must match a corpus. The PO questions are the real target. | User, handover §7. | Open. |
| 1 | Golden set review: `g09` (same section, other chunk), `g17` (Grenzwert vs. Schwellwert), `g24` (heading equals the query). Add a second expectation where another answer is also correct. | Some misses are golden-set problems, not retrieval problems. | [Experiment 0001](../experiments/0001-eval-baseline.md). | Open. Do it in a separate run: a golden change gives a new `sha256`. |
| 1 | **Eval: more golden queries.** | With 30 queries, one query is 0.033 recall. A gain of 1–2 queries is not visible. | [Experiment 0004](../experiments/0004-fusion-weights.md), finding 1. | In progress. 2026-10-19: `g34`–`g36` added from a support ticket with customer words (`g36` is the full ticket as one query). Golden set 36 queries, `golden_sha256` `1a6ebe1a…`, baseline run `20261019T130844Z` (hybrid r@1 0.30, r@5 0.82, MRR 0.518). Since [0014](../decisions/0014-eval-in-pool-status.md) the reference run is `20261019T135803Z` (same metrics, with `in_pool`). Put new candidates into `eval/golden.candidates.json` (same format, IDs `cNN`, gitignored). Promote a candidate only after an eval run on the candidates file. Open candidates: `c02`, `c03`. |
| 2 | **Data source design** (`designs/data-source-model.md`): `config.yaml` schema, sidecar `<file>.meta.json` schema, RC17 names and validation, required set (RC18 and more), system attributes, URL template, freshness policy fields, flag precedence, DB tables for metadata. | RC2–RC6, RC11 and RC15 all need this model. | [0001](../decisions/0001-config-yaml-data-sources.md), [0002](../decisions/0002-document-identity-derived-db.md). | Partly done: [sidecar-metadata](../designs/sidecar-metadata.md) (source attributes, sidecar, `classification.personal_data`, `metadata_json`). Open: typed RC17 fields, the other two RC18 attributes, URL template, freshness fields, source description. |
| 3 | **`ingest` command + RC11:** path identity, hash compare, add, replace, delete. (Old T18) | Exporters write all the time. Now a changed file adds a second document. | [0006](../decisions/0006-ingest-command.md). | Open. Remove the old extraction on replace. |
| 3 | Chunk step tolerance for a failure of one document. | One bad document stops the full run. Unattended `ingest` needs tolerance. | Design. | Open. |
| 4 | **RC5** result fields, **RC4** filters, **RC2** catalog tool, **RC3** descriptions from static config values. | Clients must see where a result comes from and which data exists. | [0004](../decisions/0004-mcp-tool-surface.md). | Partly done: RC4 filters on source and metadata and the RC2 `catalog` tool ([retrieval-filters](../designs/retrieval-filters.md)). RC5 has source, file name as title, breadcrumb and metadata. Open: RC5 URL, created and updated (need typed RC17 fields), RC3 source descriptions. |
| 5 | **RC6 read tool.** Includes the real renderer for `ChunkRenderer.RenderChunks` (old T15) and section scope by breadcrumb (old T17). | The model must go from a result to the full section. Consumers now get the index text. | Handover. | No-op renderer stub in `retrieval_renderer.go`. |
| 6 | **RC7 reranker**, **RC15 freshness policy.** Post-hydration rank stages on an over-fetched pool. | Query-dependent order, age and validity in the rank. | [0003](../decisions/0003-remote-reranker.md). | Open, not now. Measure with RC1. Run `20261019T135803Z`: hybrid has 32 of 33 correct chunks in the pool, 22 not at rank 1 (10 at rank 2), so the reranker has a large potential. Open: local rerank service and model, pool for the reranker, order of source weight and reranker. |
| 7 | **RC19a, RC19b, RC19d** OAuth resource server, identity in spans, static token in the config. | Production access with user identity. | [0005](../decisions/0005-oauth-resource-server.md). | Open. Needs the PO test realm. |
| 7 | **RC8** complete: filters, collections, delay and identity in the span of each tool call. | One trace must show a full tool call. | Handover. | Partly done (`otelhttp`, propagator, query log). |
| 8 | **RC9** test all tools with OAuth in OpenWebUI and Claude Code. | Acceptance of RC9. | Handover. | The `rag` tool works with a static token in OpenWebUI, Claude Code, Claude and Cursor. |
| 9 | **RC11** schedule and exporter trigger, **RC13** query rewrite, **RC16** delay target. | Should items. | Handover. | Open. |

## Carry-over items (no RC)

| Need | Reason | Found | Status |
|---|---|---|---|
| Size guard before the embed API. (Old T16) | Code, formula and table chunks are never split. The model can truncate silently. | Code audit. | Open. |
| Test a different model tag or quantization for chunks that get NaN from Ollama/bge-m3. (Old T16) | 16 of 1656 chunks have no embedding. | e2e run. | Open. Test before code. |
| Picture captions into chunks, also for root pictures and pictures in inline groups. (Old T19) | 785 captions drop. | e2e run. | Open. |
| Garbage-chunk filter: single-token `generic` chunks, `MISSING_TITLE_PLACEHOLDER`, OCR noise. (Old T20) | bm25 puts one-token chunks at the top. | e2e run. | Trigger: a chunk under 10 chars in an eval top-k. |
| FTS folding and synonyms (`email` = `e-mail`). (Old T21) | The tokenizer does not join these forms. | MCP use. | Open. |
| Bold and italic as a render rule. (Old T22) | Bold spans are field names that users type. | Corpus analysis. | Open. Needs phase 5. |
| Final table treatment: standalone, or merge with an intro sentence. | Open since the typed-chunk design. | Design. | Wait for a golden query on a table. |
| Handle docling `ordered_list` groups and code nodes in inline groups. | Their content drops in `walk`. | Code audit. | Count the cases first. |
| Fix the `httpclient` dump: unique file names, check dump errors, log `respFileErr`. (Old T21) | Dumps collide. Dev tooling only. | Code audit. | Open. |
| Fix OTel shutdown in `serve/cmd.go` with a new bounded context. | Each graceful stop logs "context canceled". | Code audit. | Open. |
| "Weak match" hint when no result is a dual hit. | Floor-only scores show a content gap. | Query log. | Idea. Compare with the reranker score first. |
| **Views:** `config.yaml` holds a list of views. Each view is one MCP endpoint with a fixed selection of sources and metadata (filter in SQL), later also ranking, renderer and tool texts. No UI, restart after a change ([0011](../decisions/0011-config-file-and-composition-root.md)). | Many sources make the choice of knowledge hard for the model. A narrow, fixed corpus for each endpoint gives fewer wrong answers than more complex tools. Base for a later UI with tenants. | User idea, 2026-10-19. | Idea. Details later. Needs metadata ([sidecar-metadata](../designs/sidecar-metadata.md)) and RC4 filters. A view that hides documents is an access boundary and needs tests. |

## Parked

| Need | Reason to park |
|---|---|
| Breadcrumb boost with a cap for each breadcrumb (old T14). | The reranker can make it obsolete ([0003](../decisions/0003-remote-reranker.md)). Test again after RC7 if RC1 shows a need. |
| Breadcrumb as a separate FTS column (old T23). | Same as old T14. |
| Better provenance `page` data (620 of 16780 nodes). | Docling supplies it for few items. No evidence of need. |
