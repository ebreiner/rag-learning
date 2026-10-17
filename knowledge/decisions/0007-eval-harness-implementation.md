---
type: decision
title: Eval harness as implemented
status: accepted
date: 2026-10-17
tags: [eval, rc1, design-change]
---

# 0007 Eval harness as implemented

Supersedes these parts of the locked design [eval-harness](../designs/eval-harness.md): Code layout, Run file (header), Metrics (recall levels, section key), Status (precedence).
The implementation in commit `6f840cc` is the target. The parts of the design that are not named here stay valid.

## Context

- The design was locked before the implementation.
- The implementation found small gaps and better forms. The code and the design now differ in the points below.
- The rule for a locked design: change it only with a new version or a decision.

## Options considered

1. Change the code back to the design (add the ports `GoldenSource` and `RunStore`, list the strategies in the header). More code, no gain.
2. Write a new design version. Too much text for a few points.
3. Record the differences in this decision. Accepted.

## Decision

**Code layout**
- No ports for the golden file and the run files. `internal/platform/evalstore` has the functions `LoadGolden`, `Save`, `Load` and `LoadLatest`.
  Reason: only the composition root calls them. `RunEval` gets a `GoldenSet` value and gives results. It does not read or write files.
- The ports are `Searcher` and `DocChunksProvider` (design: `DocChunks`).
- `Searcher.Search` gives `[]SearchHit`, not `[]RetrievedChunk`. `SearchHit` = `Chunk` + `Score`. `Chunk` has no score.
  Reason: chunks from `DocChunksProvider` have no retrieval score. One type for both gave a false `0` score.
- The shared wiring `WireUpRetrieval` (`cmd/cli/cmd/wiring.go`) is used by `serve` and `eval`. `retrieve` keeps its own wiring for now.
  Reason: a cheap refactor, deferred until it is necessary.
- The searcher adapter is in `cmd/cli/cmd/eval.go`. Reason: it has no SQLite, it is glue between two steps.

**Status**
- A miss gets the best status of its expectations, in the order `ranked_out` > `content_lost` > `doc_missing`.
  Reason: `ranked_out` shows that the correct content exists. That is the most useful information.
- `DocChunksProvider` reads all chunks of all documents with the name. Reason: before re-index exists, a changed file gives two documents with one name.
- The hit rank is the position in the result slice, not `RetrievedChunk.Rank`. Reason: the hydrator can skip rows.

**Metrics**
- A recall level above k is not computed and shows as `-`. Reason: a hit cannot be there.
- distinct-sections uses the key doc + breadcrumb, over the top k. Reason: two docs can have the same breadcrumb.

**Run file and difference**
- The header has no strategy list. The strategies are in the body.
- Each top-k entry has the chunk ID, doc, breadcrumb and score. The chunk text is not in the run file. Reason: file size.
- `Save` never overwrites a file (`O_EXCL`).
- The difference warns when the golden `sha256`, the embed model or dim, or k differ, or when a strategy is not in the previous run. A different git commit or chunk count gives no warning. Reason: these change on purpose between runs.

**Behaviour and output**
- A search error stops the run. Reason: a run with holes is not comparable.
- Output: metrics table, the miss list (one line for each query, strategy and status), the top 1 of each `no_answer` query, the metric deltas, the status changes.

## Consequences

- The design file stays locked and unchanged. Read it together with this decision.
- The header cannot show if the DB is older than the chunk code. A re-chunk is then not visible in the header (see the backlog).
