---
type: design
title: Sidecar metadata, first scope
status: draft
date: 2026-10-19
tags: [metadata, sidecar, source, rc14, rc18, rc5]
---

# Sidecar metadata, first scope

First part of the source design (backlog phase 2). It implements a part of [0001](../decisions/0001-config-yaml-data-sources.md) and of the [interpretation](../requirements/rag-cli-handover-interpretation.md): RC14, RC18 (partly) and RC5 (metadata in the result). The draft [unstable-scopes](unstable-scopes.md) S2 has the full target with typed RC17 fields.

## Goal

Each document gets its metadata from its source attributes and its sidecar. A document without a valid `classification.personal_data` is rejected. The metadata is stored and returned.

## Principle

The sidecar is an input detail of the source doc provider. The provider gives the extract step one finished metadata map for each document. The step only checks that map. The business logic has no sidecar concept.

## Flow

```
config sources[].attributes ─┐
                             ├─► source doc provider: merge at queue build ─► SourceDoc.Metadata ─► extract step: validate ─► metadata_json ─► result.Metadata
<file>.meta.json (optional) ─┘                                                                     (may reject)
```

## Ownership of the maps

| Map | Port | Owner and job |
|---|---|---|
| `SourceDoc.Metadata` | `DocSource` (input) | The provider fills it: the source attributes, then the sidecar keys over them. |
| the same map in `ExtractedDoc.Source` | `DocSink` (output) | The step only reads and checks it. The sink stores it unchanged. |
| `RetrievedChunk.Metadata` | `ChunkHydrator` (retrieval output) | The hydrator reads `metadata_json`. |

## Parts

| # | Part | Change |
|---|---|---|
| 1 | Config | `sources[].attributes`: a map of defaults for each document of the source. Flat keys with a dot, for example `classification.personal_data: false`. |
| 2 | Source doc provider | When it builds the queue of input docs: copy the source attributes, read `<file>.meta.json` if it exists, put its keys over the copy, and keep the result in the input doc. An invalid sidecar is kept as an error of that input doc, and `NextSourceDoc` returns it for that doc only. `*.meta.json` is not a document, and the walk gives no warning for it. One summary line: docs, docs with a sidecar, invalid sidecars. |
| 3 | Extract step | `SourceDoc.Metadata map[string]any` replaces `Additional`. `validateMetadata` checks that `classification.personal_data` exists and is a boolean. It runs after the duplicate check and before docling. A failure is an `ErrExtractionFailed`, so it counts toward the breaker of 10 failures in sequence. Revisit this after use. |
| 4 | Storage | `documents.metadata_json` (it exists). No new columns. |
| 5 | Output | A separate field `Metadata` in the retrieval result and the MCP output (RC5). |

## Rules

- **Values:** `map[string]any` from YAML and JSON. The step checks the types.
- **RC18 for now:** only `classification.personal_data`. `customer_data` and `mission_critical` come later. This is a known gap against RC18.
- **Dev corpus:** `config.yaml` sets `classification.personal_data` as a source attribute, because the corpus has no sidecars. Test sidecars exist only in the tests.
- **System attributes:** no key check. System values (source name, relative path, hash, file name, weight, IDs) are DB columns and are never read from metadata. `Metadata` is a separate output field, so a sidecar cannot hide them. This satisfies RC14.
- **Sidecar change:** it takes effect after a full re-ingest (`ingest-clean`). RC11 can change this later.
- **Duplicate content:** the first source wins, and the metadata of the second document is lost. This is a known limit ([0012](../decisions/0012-sources-table-replaces-collections.md)).

## Not in the scope (agreed)

- typed RC17 fields (title, URL, dates, language, validity, tags)
- URL template
- freshness (RC15)
- metadata filters (RC4)
- new DB columns
- RC11 (add, replace, delete)

## Acceptance

1. A sidecar value overrides the source attribute.
2. A missing or non-boolean `classification.personal_data` rejects the document with a clear message, and the next document continues.
3. An invalid sidecar fails only its own document.
4. The MCP result shows `Metadata`.
5. A full re-ingest plus `just eval` gives the same metrics as `20261019T135803Z`.
