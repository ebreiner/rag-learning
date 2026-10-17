---
type: decision
title: MCP tool surface - search, read, catalog
status: accepted
date: 2026-10-17
tags: [mcp, serve, tools, catalog]
---

# 0004 MCP tool surface: search, read, catalog

Supersedes these parts of [0000](0000-migrated-baseline.md): Serve and MCP ("one MCP tool `rag`", "section expansion stays internal, with no `expand` param").

## Context

- [Handover](../requirements/rag-cli-handover.md) RC2–RC6: a catalog, tool descriptions from the catalog, filters, rich results, a read tool.
- Now one tool `rag` (`retrieval_query`, `k`) gives hybrid results. OpenWebUI, Claude Code, Claude and Cursor use it.
- The server sends its instructions only at the MCP `initialize`. Clients cache the tool list. A live change in a description does not reach clients reliably.

## Options considered

1. Keep one tool with more params. One tool for search, read and catalog has an unclear contract.
2. Put dynamic state (document count, last ingestion) into the tool descriptions. These become stale while the server runs.
3. Separate tools. Dynamic state only through tools. Descriptions only from static config values. Accepted.

## Decision

- Tools: search, read and catalog. The names go into the design.
- Search: all collections by default. Optional filters on collection and metadata.
- Read: chunks around a result, a section by breadcrumb, a full document up to a limit. This replaces the internal section expansion.
- Catalog: the dynamic state for each data source (RC2). It reads the DB at each call.
- Tool descriptions and server instructions use only static values from `config.yaml` (data source name and description).
- A description tells the model which tool gives which state. Example: "Use the catalog tool to get the collection names and their document counts."
- A config change needs a `serve` restart. Thus descriptions are never stale.

## Consequences

- The tool `rag` goes away. The client setups (OpenWebUI, Claude Code, Claude, Cursor) must change to the new tools.
- The read tool needs the renderer (old Tier 15).
- Results need document metadata from the DB ([0001](0001-config-yaml-data-sources.md)).
