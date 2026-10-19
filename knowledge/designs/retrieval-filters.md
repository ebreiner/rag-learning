---
type: design
title: Retrieval filters on source and metadata (RC4), metadata catalog (RC2)
status: locked
date: 2026-10-19
tags: [retrieval, filter, rc4, rc2, catalog, metadata, source, mcp]
---

# Retrieval filters on source and metadata (RC4), metadata catalog (RC2)

Implements RC4 of the [handover](../requirements/rag-cli-handover.md) for sources and document metadata. It builds on [sidecar-metadata](sidecar-metadata.md) and on the two phases of [0013](../decisions/0013-retrieval-two-phases.md). RC5 needs no work here: the result already has the source, the title (file name), the breadcrumb and all metadata. URL, created and updated need the typed RC17 fields and are out of scope.

## Goal

A search can be limited to sources and to metadata values. Without a filter, nothing changes. A caller can find out which sources, metadata keys and values exist, so it can use the filters.

## Filter

```go
// internal/retrieval/step
type Filter struct {
	Sources  []string       // OR: a chunk of one of these sources; empty = all
	Metadata map[string]any // AND: each key must have exactly this value; empty = no condition
}
```

- Equality only: string, bool or number. No ranges, no NOT, no lists as values.
- A missing key does not match. A filter `enerchart_core: true` does not find a document without that key.

## Parts

| Part | Change |
|---|---|
| Port | `TopKByFTS` and `TopKByANN` get the `Filter`. `RunRetrieval` passes it on. The hydrator, the source weight and the renderer do not change. |
| FTS | Join `chunks` → `documents` with `WHERE source_name IN (…) AND json_extract(metadata_json, '$."key"') = ?`. |
| ANN | Without a filter: the KNN query as now. With a filter: a full scan of the filtered set with `vec_distance_l2`, ordered by distance, limit k. Exact. The sqlite-vec KNN query applies a `WHERE` on joined tables only after the k nearest chunks, so it cannot filter. |
| MCP `rag` | Optional `sources` (array, `enum` of the config source names) and `metadata` (object). An unknown source or a key outside `[A-Za-z0-9_.-]` gives a tool error. The key check also protects the JSON path in the SQL. |
| CLI `retrieve` | `--source` (more than one permitted) and `--meta key=value`. |
| Telemetry | The span and the query log record the filter (part of RC8). |
| Eval | No filter, no change. |

## Metadata catalog

A cheap catalog of the sources and the metadata that exist, for the caller of the filters. No new table, no change to the ingest.

**Source of the data:** `documents.metadata_json`, read with one query:

```sql
SELECT d.source_name, j.key, j.type, j.value, COUNT(*) AS documents
FROM documents AS d, json_each(d.metadata_json) AS j
GROUP BY d.source_name, j.key, j.type, j.value;
```

**Snapshot at start:** `serve` reads the catalog once at start and keeps it in memory. `serve` restarts after each ingest ([0011](../decisions/0011-config-file-and-composition-root.md)), so the snapshot is always correct. A tool call needs no DB access.

**Shape** (domain type in `internal/retrieval/step`, filled by a SQLite adapter):

```go
type Catalog struct {
	Sources []CatalogSource // name, document count, keys of this source
}

type CatalogKey struct {
	Key            string
	Types          []string       // boolean, integer, real, text, object, array, null
	Documents      int64          // documents of the source with this key
	DistinctValues int
	Values         []CatalogValue // value and document count; empty above the value cap and for objects and arrays
}
```

The keys are listed for each source, because a filter combines a source with metadata.

**Value cap:** a key lists its values only up to 20 distinct values. Above the cap, the catalog gives only the key, the type and the document count. Thus keys like `title` or dates do not make the catalog large.

**Consumers:**
- An MCP tool `catalog` (RC2, as planned in [0004](../decisions/0004-mcp-tool-surface.md): dynamic state only through tools). It has one required parameter `source`, with an `enum` of the config source names, and gives the keys and values of that source. The source names are static config values and change only with a restart, so the tool description also names them. The model reads the sources in the description, calls `catalog` for one source, then calls `rag` with the filter. A configured source without documents gives an empty entry.
- A CLI command `catalog` that prints the same catalog as JSON, for all sources or with `--source` for one source. It is the counterpart of `retrieve --source/--meta` for tests by hand. It reads the DB at each call, because it has no long-lived process. It adds a fifth command to the four of [0011](../decisions/0011-config-file-and-composition-root.md).

## Not in the scope

- Descriptions of the metadata keys.
- Ranges, NOT, lists as values.
- A metadata table with an index (`document_metadata`). Revisit only when `json_extract` becomes too slow for the corpus size.

These wait for the views (backlog). Build then what the views need.

## Acceptance

1. FTS and ANN tests: source filter, metadata filter with bool, string and number, missing key. Errors for an unknown source and an invalid key.
2. A filter on all sources gives the same order as no filter.
3. `just eval` without a filter is equal to `20261019T135803Z`.
4. `retrieve --meta enerchart_core=false` gives only chunks of the document with that sidecar value.
5. Catalog tests: counts for each source and key, values up to the cap, only key, type and count above the cap, documents without metadata.
6. The `catalog` tool and the `catalog` command on the dev corpus show `enerchart_core` with `true: 198, false: 1` for `manual`.
