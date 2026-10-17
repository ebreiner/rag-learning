---
type: decision
title: Remote reranker, collection weight stays optional
status: accepted
date: 2026-10-17
tags: [retrieval, ranking, reranker]
---

# 0003 Remote reranker, collection weight stays optional

Supersedes this part of [0000](0000-migrated-baseline.md): Retrieval and ranking ("No cross-encoder or Python rerank stack").

## Context

- [Handover](../requirements/rag-cli-handover.md) RC7: a reranker with a query-dependent order. A remote API is acceptable. It can be switched off. It replaces or adds to the fixed collection weights.
- Now the hybrid order is RRF × collection weight. The weight does not depend on the query.

## Options considered

1. No reranker, only heuristics (old Tier 14 breadcrumb boost). Does not satisfy RC7.
2. Local in-process model (ONNX or a Python stack). More build and run complexity.
3. Remote rerank API, called after hydration on an over-fetched pool. Accepted.

## Decision

- Add an optional remote reranker. It runs after hydration on a pool larger than `k`. A config value switches it on and off.
- The fixed collection weight stays as an optional data source value.
- RC1 numbers decide how the weight and the reranker combine, and which API and model to use.

## Consequences

- A remote call adds delay to each search. RC16 measures it.
- Old Tier 14 (breadcrumb boost) and Tier 23 (breadcrumb FTS column) are parked. Test them again only if RC1 shows a need after the reranker.
- The RC15 freshness policy is a different rank stage after hydration. Its order relative to the reranker goes into the design.
