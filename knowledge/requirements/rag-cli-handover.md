---
type: requirement
title: Handover to team rag-cli
status: locked
date: 2026-10-17
tags: [rag-cli, handover, product-owner]
---

# Handover to team rag-cli

From: the product owner project. To: team rag-cli (engineers).
Put this file into `knowledge/requirements/` of rag-cli, unchanged.

## 1. Goal

- Users chat with an LLM in OpenWebUI, Claude-Code and ChatGPT. The chat must give the correct knowledge without a manual selection. Users often do not know what knowledge exists.
- The same knowledge must be available to local clients (Claude Code, experiments).
- Knowledge is the main value. It is mission critical.

## 1a. Users and license

- rag-cli is open source under MIT. It stays MIT.
- All differences in instances go into `config.yaml`.

## 2. Decisions on the product owner side

- Models through OpenRouter.
- No router before the model call. The routing happens inside the rag-cli tools.
- rag-cli is the knowledge server. Clients use it only through MCP.

## 3. How rag-cli fits

```
source systems ──exporters──► central directory (docs + JSON sidecars)
                                   │
                              rag-cli ingest
                                   │
OpenWebUI ──MCP (OAuth)──► rag-cli serve ◄──MCP (OAuth)── Claude Code / local
                                   │
                              OTel traces
```

- Exporters are outside rag-cli. They are the only writers to the central directory. They write each document plus one JSON sidecar with its metadata.
- rag-cli has no source handlers.

## 4. Requirements

Must:

| ID | Requirement | Accepted when |
|---|---|---|
| RC1 | Golden query set and one quality command. Queries German. Documents German and English. | Metrics for each run plus the difference to the last run. |
| RC2 | Catalog through MCP: for each data source name, description, document count, last ingestion, count without URL, metadata keys in use. | A client lists it. |
| RC3 | Tool descriptions and server instructions come from the catalog. | A new data source changes them with no code change. |
| RC4 | Search all collections by default. Optional filters: collection, metadata. | A filter changes the results. |
| RC5 | Each result: collection, title, breadcrumb, source URL if available, created and updated if available, all document metadata. | Results without URL are marked. |
| RC6 | Read tool: chunks around a result, section by breadcrumb, full document up to a limit. | The model goes from a result to the full section. |
| RC7 | Reranker, order depends on the query. Remote API is acceptable. Can be switched off. Replaces or adds to the fixed collection weights. | RC1 shows the effect. |
| RC8 | Telemetry for each tool call: query, filters, collections, result IDs, scores, delay. Continues the trace context of the client. | One trace shows a full tool call. |
| RC9 | Works with native MCP in OpenWebUI (Streamable HTTP) and with Claude Code. | Both clients use all tools. |
| RC10 | `config.yaml` with many data sources. Each: recursive directory, name, description, attributes for all its documents (like OTel resource attributes), optional URL template, freshness policy. | Two data sources with different attributes ingest correctly. |
| RC14 | One JSON sidecar for each input file, all file types. Sidecar overrides data source attributes. System-controlled attributes cannot be overridden. | A sidecar key overrides the data source attribute. |
| RC15 | Freshness policy for each data source changes the rank by age. An explicit validity status has priority over age. Dates only from the exporter, never from file times. Unknown date: no effect. | RC1 shows the effect. Policy "none": no effect. |
| RC17 | Known meanings that rag-cli interprets: source URL, title, created, updated, language, validity status, tags, classification. Team rag-cli defines names and validation. | Documented and validated. |
| RC18 | Three boolean classification attributes for each document: customer data (GDPR), personal / user data, mission critical. Missing or invalid value: hard error, the document is rejected. Store and return them. Do not interpret them. | A document without valid values fails the ingestion. |
| RC19a | OAuth resource server as the MCP spec requires. Checks JWTs from a configured OIDC issuer (Keycloak). Publishes Protected Resource Metadata (RFC 9728). Answers 401 or serves. Issues no tokens, keeps no users. | No token: 401 with the metadata location. Valid token: access. |
| RC19b | Identity from the token goes into telemetry. | Each trace shows the user. |
| RC19d | Static token stays as a config option for development. | Both modes work. |

Should:

| ID | Requirement |
|---|---|
| RC11 | Scheduled re-ingestion with hash comparison, plus a trigger for exporters. Changed and deleted documents are updated. |
| RC13 | Query rewrite on the server. Can be switched on and off. Stays only if RC1 shows a gain. |
| RC16 | 95 % of tool calls take less than 5 s. Measure and optimize at the end. |

## 5. Non-goals

- Access control. Policy on RC18 attributes. Rich Authorization Requests.
- UI, chat, router before the model call.
- Source handlers. S3 input (directory only for now).
- English user queries.

## 6. Known unknowns

- Test Keycloak realm for RC19
- OAuth flow of OpenWebUI against Keycloak: not verified
- MCP spec: 2025-11-25 adds CIMD. A 2026-07-28 page says DCR is deprecated. Not verified which is released. Client registration is between client and Keycloak, not rag-cli.
- Docling and wikitext: probably not supported. Exporters convert to Markdown or HTML. Not verified.

## 7. What the product owner supplies

- Data sources with descriptions and metadata, through exporters.
- Real German user questions with the correct sources, for RC1.
- OpenWebUI presets, system prompts, Claude Code setup.
- Test reports with telemetry as evidence. New or changed needs come as a new handover version.

