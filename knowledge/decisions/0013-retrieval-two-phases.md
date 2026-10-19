---
type: decision
title: Retrieval in two phases, the source weight after hydration
status: accepted
date: 2026-10-19
tags: [retrieval, ranking, source, fusion, reranker]
---

# 0013 Retrieval in two phases, the source weight after hydration

Adds to [0003](0003-remote-reranker.md), [0009](0009-fusion-config-default.md) and [0012](0012-sources-table-replaces-collections.md). Supersedes these parts of [0009](0009-fusion-config-default.md): the built-in fusion default in the domain (`withDefaults`). Changes the S1 section of the draft design [unstable-scopes](../designs/unstable-scopes.md): the `RankStage` interface comes later, with the reranker.

## Context

- `runHybrid` applies the source weight on chunk IDs, before hydration. It uses a separate port `SourceWeigher` with its own DB query. The hydrator loads the same weight again.
- The next ranking signals need data that exists only after hydration: the reranker (RC7) needs the chunk text, and freshness (RC15) needs dates from the metadata. With the current structure, each signal needs a new port, a new query and more code in `runHybrid`.
- A miss in the eval does not show which part failed. The correct chunk can be outside the candidates, or it can be in the candidates but ranked too low. Each case needs a different fix.
- `FusionConfig.withDefaults()` changes a weight of 0 to 1. The config validation permits `weight_fts: 0` with the meaning "hybrid uses only ANN". Thus a valid config value has no effect.

## Options considered

1. Keep the weight on IDs before hydration. Add a port for each new signal.
2. Two phases with the full stage chain of the S1 draft now: `Candidate`, `RankStage` interface, stage list in the config.
3. Two phases, lean: a pool, then hydration, then the source weight as a plain function. The `RankStage` interface comes with the second stage (the reranker). Accepted.

## Decision

- **Phase 1, find (recall):** FTS and ANN each give `pool_size` IDs. Fusion (weighted RRF) merges them. The result is cut to `pool_size`. This phase uses only indexes and IDs.
- **Phase 2, order (precision):** the pool is hydrated. The source weight multiplies the score of each hydrated chunk. The result is sorted again and cut to `k`.
- **Pool size:** `retrieval.pool_size` in the config, required, > 0. If `k` is larger than the pool, the pool is `k`.
- **Hybrid only:** the source weight applies only to the `hybrid` strategy. `fts` and `ann` stay raw index signals. Thus the eval shows the quality of each index (`fts`, `ann`) and the quality of the product (`hybrid`).
- **Output:** each result has `SourceName` and `SourceWeight`.
- **No fusion defaults in the domain:** `RunRetrieval` uses `FusionConfig` as given. `config.Load` is the only source of the values, and `validate` checks them.
- **No stage abstraction yet:** the `SourceWeigher` port goes away. The `RankStage` interface and a stage list in the config come with the reranker (RC7), when a second stage exists.

## Consequences

- Each new ranking signal goes after hydration and reads the hydrated chunk. It needs no new port and no new query.
- The hybrid ranking can change a little. A chunk outside the top `pool_size` by RRF can no longer move into the top `k` because of its source weight. Compare the first eval run after this change with the baseline.
- Hydration reads `pool_size` chunks in place of `k`. This is one SQL query with an `IN` list.
- A later eval change can report "in the pool, but not in the top k" as a separate status. That status shows when a reranker can help.
- The MCP output field names changed from `CollectionName` and `CollectionWeight` to `SourceName` and `SourceWeight`. Restart `serve` to get them.
- Evidence for the rename part: eval run `20261019T134044Z` has a `strategies` section that is byte-equal to the baseline `20261019T130844Z`.
