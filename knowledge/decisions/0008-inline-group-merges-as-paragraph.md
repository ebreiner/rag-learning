---
type: decision
title: A docling inline group merges like a paragraph
status: accepted
date: 2026-10-18
tags: [chunk, generic, merge, cycle-1]
---

# 0008 A docling inline group merges like a paragraph

Supersedes these parts of [0000](0000-migrated-baseline.md): chunk step ("group → one candidate", "`mergeCandidates`: only paragraph candidates merge", type `generic`) and the known limit "Non-content chunks have no breadcrumb" for groups.

## Context

- Docling cuts one paragraph into an inline group at each bold or italic span. The chunk step made each group its own `generic` chunk, without a breadcrumb in the index text.
- [Experiment 0001](../experiments/0001-eval-baseline.md): 66 % of all chunks were `generic`. All 5 full misses were `generic` chunks.
- Evidence from cycle 1 of `claude-unstable` (tag `unstable/1/end`):
  - [Experiment 0002](../experiments/0002-generic-breadcrumb.md), commit `653f393`: breadcrumb prefix only.
  - [Experiment 0003](../experiments/0003-generic-merge.md), commit `e336bbd`: merge like a paragraph.

## Options considered

1. Breadcrumb prefix for `generic` chunks only (S0.1). Hybrid r@3 +0.13, r@5 +0.10. But ANN r@1 −0.07 and distinct-sections@10 −0.7: in short chunks, the breadcrumb dominates the embedding, and siblings look too similar.
2. Merge a group like a paragraph (S0.2, on top of option 1). Hybrid r@5 0.60 → 0.80, MRR 0.408 → 0.503 against the base. No strategy is worse than the base at any level. The side effects of option 1 are gone. Accepted.
3. Option 2 and keep the prefix of option 1 for groups that do not merge. Not necessary: after option 2, no `generic` chunks remain.

## Decision

- A group candidate goes into the content merge buffer under the same rules as a paragraph: same breadcrumb, budget 1000, flush on a breadcrumb change or an overflow.
- A group adds all its member nodes to the chunk, each with the next position.
- The result type is `content`. The normal flush adds the breadcrumb prefix. The S0.1 mechanism is discarded.

## Consequences

- Chunks for the test corpus: 1806 → 1116. Types after the change: `content`, `list`, `table`, `code`. The type `generic` is no longer made for groups.
- The user writes the change on `master` again. The acceptance test: a full re-import and `just eval` give exactly the numbers of run `20261017T164359Z` (1116 chunks, hybrid r@5 0.80, MRR 0.503). The pipeline is deterministic, so a difference shows a difference in the code.
- Most remaining misses are "correct document, other chunk". The backlog gets a document-level hit rate.
