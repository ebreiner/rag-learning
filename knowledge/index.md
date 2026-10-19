---
type: index
title: Knowledge base index
status: current
date: 2026-10-19
tags: [index]
---

# Knowledge base index

Start here:
1. Read the [handover](requirements/rag-cli-handover.md) for the product goal and the requirements.
2. Read [0000](decisions/0000-migrated-baseline.md) for the current architecture. Decisions 0001–0014 supersede or add to parts of it.
3. Read the [backlog](requirements/backlog.md) for the work order.

## Decisions

- [0000-migrated-baseline](decisions/0000-migrated-baseline.md): architecture before the handover, settled decisions, known limits and bugs. Partly superseded by 0001–0006.
- [0001-config-yaml-data-sources](decisions/0001-config-yaml-data-sources.md): `config.yaml` with data sources, sidecar overrides, required attributes without built-in defaults.
- [0002-document-identity-derived-db](decisions/0002-document-identity-derived-db.md): identity = (data source, relative path), hash = change test, DB is derived, no migrations.
- [0003-remote-reranker](decisions/0003-remote-reranker.md): optional remote reranker, collection weight stays optional.
- [0004-mcp-tool-surface](decisions/0004-mcp-tool-surface.md): search, read and catalog tools, dynamic state only through tools.
- [0005-oauth-resource-server](decisions/0005-oauth-resource-server.md): JWT check, RFC 9728 metadata, static token for development.
- [0006-ingest-command](decisions/0006-ingest-command.md): one `ingest` command with add, replace and delete.
- [0007-eval-harness-implementation](decisions/0007-eval-harness-implementation.md): the eval implementation is the target where it differs from the locked eval design.
- [0008-inline-group-merges-as-paragraph](decisions/0008-inline-group-merges-as-paragraph.md): a docling inline group merges like a paragraph, no more `generic` chunks for groups (cycle 1).
- [0009-fusion-config-default](decisions/0009-fusion-config-default.md): `FusionConfig` port, default RRF k 60 and weights 1/1, no tuning with the current golden set (cycle 1).
- [0010-source-single-term](decisions/0010-source-single-term.md): "source" replaces "collection" and "data source" in all layers; code rename in S2.
- [0011-config-file-and-composition-root](decisions/0011-config-file-and-composition-root.md): one YAML file via `--config`, config values go into adapters at the composition root, commands `ingest`/`serve`/`eval`/`retrieve`, restart after ingest, secrets in the config for now.
- [0012-sources-table-replaces-collections](decisions/0012-sources-table-replaces-collections.md): table `sources` (name, weight), `documents.source_name` FK and `rel_path`, first source wins for duplicate content.
- [0013-retrieval-two-phases](decisions/0013-retrieval-two-phases.md): find (FTS+ANN, fusion, pool) then order (hydrate, source weight, cut to k); weight in hybrid only, no fusion defaults, `RankStage` interface later with the reranker.
- [0014-eval-in-pool-status](decisions/0014-eval-in-pool-status.md): eval status `in_pool` (rank k+1 to `pool_size`), `ranked_out` = not in the pool, reference run `20261019T135803Z`.

## Designs

- [eval-harness](designs/eval-harness.md): RC1 eval command, golden format, status for each query, metrics, run files outside the DB. Locked, partly superseded by 0007.
- [unstable-scopes](designs/unstable-scopes.md): scopes S0–S6 for `claude-unstable`, ports for each scope, cycle plan. Draft.

## Experiments

- [0001-eval-baseline](experiments/0001-eval-baseline.md): first valid eval baseline (1806 chunks), findings: `generic` chunks without section signal, hybrid vs. ANN at rank 1, golden-set candidates.
- Cycle 1 of `claude-unstable` (tag `unstable/1/end`):
  - [0002-generic-breadcrumb](experiments/0002-generic-breadcrumb.md): breadcrumb prefix for `generic` chunks; recall gain, but ANN rank-1 loss and less diversity.
  - [0003-generic-merge](experiments/0003-generic-merge.md): groups merge like paragraphs; 1116 chunks, hybrid r@5 0.80, MRR 0.503. Basis of 0008.
  - [0004-fusion-weights](experiments/0004-fusion-weights.md): RRF weight grid; no gain over equal weights. Basis of 0009.

## Requirements

- [rag-cli-handover](requirements/rag-cli-handover.md): handover from the PO, unchanged, locked.
- [rag-cli-handover-interpretation](requirements/rag-cli-handover-interpretation.md): gap for each RC, agreed interpretation, open questions.
- [backlog](requirements/backlog.md): open work in phases, ordered by RC IDs, plus carry-over items.
