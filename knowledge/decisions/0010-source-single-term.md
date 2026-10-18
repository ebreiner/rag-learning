---
type: decision
title: Source is the single term for the origin of a document
status: accepted
date: 2026-10-18
tags: [naming, source, data-source, collection]
---

# 0010 Source is the single term for the origin of a document

Adds to [0001](0001-config-yaml-data-sources.md), [0002](0002-document-identity-derived-db.md), [0003](0003-remote-reranker.md), [0004](0004-mcp-tool-surface.md) and [0006](0006-ingest-command.md). These decisions use "data source" and "collection". Their content does not change, only the term.

## Context

- The [handover](../requirements/rag-cli-handover.md) uses "data source" and "collection" for the same thing. The code uses "collection" (`collection-name`, `collection-weight`, `collectionRerank`).
- The directory walk package `internal/platform/extraction/source` already uses "source" for a different thing.
- Two terms for one thing, and one term for two things, make the design of the source model (S2) unclear.
- The naming section of the draft design [unstable-scopes](../designs/unstable-scopes.md) has the proposal.

## Options considered

1. "collection": the current code term. But the handover and decision 0001 use "data source".
2. "data source": the handover term. Two words, and no good identifier form (`dataSource`, `data_sources`).
3. "source": short, one word, and the handover term without "data". Accepted.

## Decision

- "source" replaces "collection" and "data source" in all layers: table `sources`, column `documents.source`, config key `sources:`, domain type `source.Source`, MCP filter and result field `source`, span attribute `rag.source`.
- The directory walk package `internal/platform/extraction/source` gets the new name `internal/platform/extraction/walk`.
- New text in `knowledge/` uses "source". Old decisions stay as they are (append-only). Read "data source" and "collection" there as "source".

## Consequences

- The rename of the code is part of scope S2 (source model) in cycle 2. No separate rename before that.
- The CLI flags `--collection-name` and `--collection-weight` change with the `ingest` command and `config.yaml` ([0006](0006-ingest-command.md), [0001](0001-config-yaml-data-sources.md)).
