---
type: index
title: Knowledge base index
status: current
date: 2026-10-17
tags: [index]
---

# Knowledge base index

Start here:
1. Read the [handover](requirements/rag-cli-handover.md) for the product goal and the requirements.
2. Read [0000](decisions/0000-migrated-baseline.md) for the current architecture. Decisions 0001–0006 supersede parts of it.
3. Read the [backlog](requirements/backlog.md) for the work order.

## Decisions

- [0000-migrated-baseline](decisions/0000-migrated-baseline.md): architecture before the handover, settled decisions, known limits and bugs. Partly superseded by 0001–0006.
- [0001-config-yaml-data-sources](decisions/0001-config-yaml-data-sources.md): `config.yaml` with data sources, sidecar overrides, required attributes without built-in defaults.
- [0002-document-identity-derived-db](decisions/0002-document-identity-derived-db.md): identity = (data source, relative path), hash = change test, DB is derived, no migrations.
- [0003-remote-reranker](decisions/0003-remote-reranker.md): optional remote reranker, collection weight stays optional.
- [0004-mcp-tool-surface](decisions/0004-mcp-tool-surface.md): search, read and catalog tools, dynamic state only through tools.
- [0005-oauth-resource-server](decisions/0005-oauth-resource-server.md): JWT check, RFC 9728 metadata, static token for development.
- [0006-ingest-command](decisions/0006-ingest-command.md): one `ingest` command with add, replace and delete.

## Requirements

- [rag-cli-handover](requirements/rag-cli-handover.md): handover from the PO, unchanged, locked.
- [rag-cli-handover-interpretation](requirements/rag-cli-handover-interpretation.md): gap for each RC, agreed interpretation, open questions.
- [backlog](requirements/backlog.md): open work in phases, ordered by RC IDs, plus carry-over items.
