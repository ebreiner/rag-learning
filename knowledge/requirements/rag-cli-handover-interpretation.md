---
type: requirement
title: Interpretation of the rag-cli handover
status: open
date: 2026-10-17
tags: [rag-cli, handover, interpretation, gap-analysis]
---

# Interpretation of the rag-cli handover

- Source: [rag-cli-handover](rag-cli-handover.md) (from the PO, locked). This file is the interpretation of team rag-cli.
- Baseline: [0000](../decisions/0000-migrated-baseline.md). Work order: [backlog](backlog.md).

## Gap for each requirement

| ID | State on 2026-10-17 | Size |
|---|---|---|
| RC1 | Tier 13 of the old backlog. Golden set drafted (`eval/golden.json`), no runner. | M |
| RC2, RC3 | No catalog. `collections` has only `name` and `weight`. | M |
| RC4 | One tool `rag`, no filters. No document metadata is stored. | M |
| RC5 | `DocTitle` is the file name. No URL, no dates, no metadata. | M |
| RC6 | Not built. Needs the renderer (old Tier 15) and section expansion (old Tier 17). | L |
| RC7 | Not built. | M |
| RC8 | Partly done: `otelhttp` + `TraceContext` propagator, query log, span attributes. Missing: filters, collections, delay, identity. | S |
| RC9 | The `rag` tool works with OpenWebUI (native MCP, static token through a static header), Claude Code, Claude and Cursor. OAuth not tested. | S |
| RC10, RC14, RC17, RC18 | Not built. Collections come from CLI flags. | L |
| RC11 | Not built. A changed file adds a second document. Delete does not exist. | L |
| RC15 | Not built. | M |
| RC19a, RC19b | Not built. | M |
| RC19d | Done (`RAG_CLI_API_TOKEN`). Must move into the config. | S |
| RC13, RC16 | Not built (should). | M |

## Decisions from the handover

Agreed with the user on 2026-10-17. Each one supersedes a part of 0000:

- [0001](../decisions/0001-config-yaml-data-sources.md): `config.yaml` with data sources (RC10, RC14, RC17, RC18).
- [0002](../decisions/0002-document-identity-derived-db.md): document identity by path, the DB is a derived artifact (RC11).
- [0003](../decisions/0003-remote-reranker.md): remote reranker, collection weight stays optional (RC7).
- [0004](../decisions/0004-mcp-tool-surface.md): search, read and catalog tools. State only through tools (RC2–RC6).
- [0005](../decisions/0005-oauth-resource-server.md): OAuth resource server, static token for development (RC19).
- [0006](../decisions/0006-ingest-command.md): one `ingest` command (RC10, RC11).

## Interpretation (agreed with the user)

- **Attribute defaults (RC14, RC18):** data source attributes are defaults for each document. A sidecar value overrides them. rag-cli has no built-in default for the classification attributes.
- **Required set:** rag-cli defines a set of required attributes. Each document must get a value for each one, from its sidecar or from an explicit data source attribute. Else the document is rejected with a hard error. The RC18 classification attributes are in the set. The full set goes into the data source design.
- **Sidecar name:** `<file>.meta.json`, next to the input file. The source walk must not ingest sidecars as documents.
- **RC3 boundary:** see 0004. Tool descriptions use only static values from `config.yaml`. Dynamic state comes only from tools.
- **RC7 weight:** the fixed collection weight stays as an optional data source value. RC1 decides how weight and reranker combine.
- **Wikitext:** the exporter converts it to Markdown with pandoc. No rag-cli work.
- **Golden set:** each installation has its own corpus. The golden set path must come from the config. The PO supplies the real German questions later (handover §7).

## Open questions

| Question | For | Status |
|---|---|---|
| Which attributes are in the required set beyond the RC18 classification? | Team rag-cli | Open, data source design. |
| Names, types and validation of the RC17 known meanings. | Team rag-cli | Open, data source design. |
| Which reranker API and model? | Team rag-cli | Open, decide with RC1 numbers. |
| Test Keycloak realm for RC19. | PO | Open (handover §6). |
| OAuth flow of OpenWebUI against Keycloak. | PO | Open (handover §6). |
| MCP spec version, CIMD vs. DCR. | PO, client side | Open. Client registration is not in rag-cli. |
