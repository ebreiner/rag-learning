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
| 1 | **S0.2 on master:** a docling inline group merges like a paragraph. See [0008](../decisions/0008-inline-group-merges-as-paragraph.md). | 66 % of the chunks were `generic` without section signal. Hybrid r@5 0.60 → 0.80 in [experiment 0003](../experiments/0003-generic-merge.md). | Cycle 1 of `claude-unstable`, commit `e336bbd`. | Open. The user writes it again. Acceptance: a full re-import and `just eval` give exactly run `20261017T164359Z` (1116 chunks, hybrid r@5 0.80, MRR 0.503). |
| 1 | **`FusionConfig` port** with the default k 60, weights 1/1. See [0009](../decisions/0009-fusion-config-default.md). | Fusion tests without code changes. The reranker stages come on the same path. | Cycle 1, commit `3765337`, [experiment 0004](../experiments/0004-fusion-weights.md). | Open. The user writes it again. Acceptance: `just eval` gives the same numbers as before the change. |
| 1 | **Eval: document-level hit rate.** A query is a doc hit when a chunk of an expected document is in the top k. | Most misses after S0.2 are "correct document, other chunk". The chunk-level status hides them. Cycle 2 changes ingest, and a doc-level metric shows those effects. | [Experiment 0003](../experiments/0003-generic-merge.md), finding 4. | Open. Before the fork of cycle 2. Do it after S0.2 on master, so that the two effects stay separate. |
| 1 | **Eval: more golden queries.** | With 30 queries, one query is 0.033 recall. A gain of 1–2 queries is not visible. | [Experiment 0004](../experiments/0004-fusion-weights.md), finding 1. | In progress. 2026-10-19: `g34`–`g36` added from a support ticket with customer words (`g36` is the full ticket as one query). Golden set 36 queries, `golden_sha256` `1a6ebe1a…`, new baseline run `20261019T130844Z` (hybrid r@1 0.30, r@5 0.82, MRR 0.518). Compare later runs with this run, not with older runs. Put new candidates into `eval/golden.candidates.json` (same format, IDs `cNN`, gitignored). Promote a candidate only after an eval run on the candidates file. Open candidates: `c02`, `c03`. |
| 2 | **Data source design** (`designs/data-source-model.md`): `config.yaml` schema, sidecar `<file>.meta.json` schema, RC17 names and validation, required set (RC18 and more), system attributes, URL template, freshness policy fields, flag precedence, DB tables for metadata. | RC2–RC6, RC11 and RC15 all need this model. | [0001](../decisions/0001-config-yaml-data-sources.md), [0002](../decisions/0002-document-identity-derived-db.md). | Open. |
| 3 | **`ingest` command + RC11:** path identity, hash compare, add, replace, delete. (Old T18) | Exporters write all the time. Now a changed file adds a second document. | [0006](../decisions/0006-ingest-command.md). | Open. Remove the old extraction on replace. |
| 3 | Chunk step tolerance for a failure of one document. | One bad document stops the full run. Unattended `ingest` needs tolerance. | Design. | Open. |
| 4 | **RC5** result fields, **RC4** filters, **RC2** catalog tool, **RC3** descriptions from static config values. | Clients must see where a result comes from and which data exists. | [0004](../decisions/0004-mcp-tool-surface.md). | Open. Needs phase 2. |
| 5 | **RC6 read tool.** Includes the real renderer for `ChunkRenderer.RenderChunks` (old T15) and section scope by breadcrumb (old T17). | The model must go from a result to the full section. Consumers now get the index text. | Handover. | No-op renderer stub in `retrieval_renderer.go`. |
| 6 | **RC7 reranker**, **RC15 freshness policy.** Post-hydration rank stages on an over-fetched pool. | Query-dependent order, age and validity in the rank. | [0003](../decisions/0003-remote-reranker.md). | Open. Measure with RC1. |
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
| Replace the Cobra placeholder `Short`/`Long` text. | The help output is wrong. | Code audit. | Open. |
| "Weak match" hint when no result is a dual hit. | Floor-only scores show a content gap. | Query log. | Idea. Compare with the reranker score first. |

## Parked

| Need | Reason to park |
|---|---|
| Breadcrumb boost with a cap for each breadcrumb (old T14). | The reranker can make it obsolete ([0003](../decisions/0003-remote-reranker.md)). Test again after RC7 if RC1 shows a need. |
| Breadcrumb as a separate FTS column (old T23). | Same as old T14. |
| Better provenance `page` data (620 of 16780 nodes). | Docling supplies it for few items. No evidence of need. |
