---
type: design
title: Document links, stage 1 (version 2)
status: locked
date: 2026-10-22
tags: [links, graph, sidecar, retrieval, mcp, rc5]
---

# Document links, stage 1

Backlog phase 1. An exporter writes the outgoing links of each document into its sidecar. rag-cli stores them, computes the incoming links, and shows both in the `rag` result. The model opens a linked document with the existing `read` tool. There is no graph engine and no change to the rank. Stage 2 (anchor text as a search list) and stage 3 are in the [backlog](../requirements/backlog.md).

It extends [sidecar-metadata](sidecar-metadata.md). It uses the filter of [retrieval-filters](retrieval-filters.md) and the `chunk_id` entry of [read-tool](read-tool.md).

## Goal

1. A link that a person wrote goes from the source to the model with no loss.
2. Each document in a `rag` result shows its outgoing and incoming links, one time for each response.
3. The model opens a link target with `read {chunk_id, mode}`. It needs no new tool.

## Facts (wiki test export, 2026-10-21)

| | Value |
|---|---|
| Pages written | 525 |
| Link entries between written pages | 570 (one entry for each target and section), 167 with anchor text, 529 with a section |
| Pages with incoming links | 336 |
| Out-degree | most pages 0. Three hub pages: hub A 62, hub B 40, hub C 33 |
| In-degree | max. 19 |

The throwaway exporter `../rag-wikimeta/tools/wikimeta/wikimeta.py` (not committed) writes this data.

**The hubs sort their links under headings.** The heading is the human signal, not the number of links:

| Hub | Links under each heading |
|---|---|
| Hub A (product page) | internal docs 18, review needed 37, obsolete 2, rework needed 1, customer resources 4 |
| Hub B (topic page) | current 5, review needed 29, obsolete 4, rework needed 2 |
| Hub C (process list) | 8 process groups, for example administration 11, software development 7, product 4 |

Thus a cut by list order is wrong: the process list is the most important hub, and hub A lists 37 pages for review. The output groups the links by section (below).

## Sidecar contract

One reserved top-level key `links`. All other sidecar rules stay the same.

```json
{
  "classification.personal_data": false,
  "doc.title": "Process list",
  "links": [
    {"to": "Feature workflow.md"},
    {"to": "Server migration.md", "text": "migration", "section": "Internal docs"}
  ]
}
```

| Field | Rule |
|---|---|
| `to` | Required. The `rel_path` of the target in the **same source**, relative to the source root, in the form of `documents.rel_path`. No absolute path, no `..`. |
| `text` | Optional. The anchor text. Max. 300 characters. Stage 2 uses it. |
| `section` | Optional. The heading path of the link in the document that contains it, for example `Internal docs > Review needed`. Separator ` > `, without the document title. Max. 300 characters. rag-cli does not interpret it: it shows it and groups by it. |
| other fields | Rejected. A typo must not disappear without a message. |

**`doc.title` is required with `links`.** A sidecar with a `links` key must have `doc.title` as a non-empty string, else the document is rejected. The Markdown of the document starts with `# <doc.title>`, with the same text. rag-cli uses `doc.title` to show a link and to remove the title from the chunk breadcrumb (see the output rules). A source that wants links supplies titles. A link target needs no `doc.title`: without it, rag-cli shows its document name.

**Rules:**

- The list order is the order in the source. rag-cli keeps it.
- A link to the document itself is dropped. A second entry with the same `to` and `section` is dropped. The first entry wins. The same target in two sections gives two rows, because each section is a separate human statement.
- Max. 1000 entries for each document. More entries make the document invalid.
- `links` in the source `attributes` of the config is a config error. Links belong to one document only.
- A target that does not exist is not an error. It is resolved at query time and is not shown while it does not exist.
- Cross-source links (a `source` field) are not in stage 1.

## Flow

```
<file>.meta.json ─► source doc provider: take "links" out of the map ─► SourceDoc.Links ─► extract step: validate ─► sink: document_links rows (same transaction as the document)
                                         (shape error = sidecar error of that doc)                (rule error = ErrInvalidMetadata)
```

| Part | Change |
|---|---|
| Source doc provider | Remove `links` from the merged map and parse it into `SourceDoc.Links []Link`. A wrong JSON shape is an invalid sidecar of that document, as today. Thus `links` never goes into `metadata_json`, and the catalog and the filters do not change. |
| Extract step | `validateLinks` next to `validateMetadata`: the field rules above and the `doc.title` rule. A failure is an `ErrInvalidMetadata` (counts toward the breaker). It drops self links and duplicates. |
| Sink | `SaveExtractedDoc` writes the links in the same transaction as the document. `to_source` is the source of the document. |
| RC11 (later) | A replace deletes the old rows (cascade) and writes the new ones. A sidecar change must count as a document change. |

## Storage

```sql
CREATE TABLE IF NOT EXISTS document_links (
  from_document_id INTEGER NOT NULL,
  position INTEGER NOT NULL,          -- order in the sidecar list
  to_source TEXT NOT NULL,
  to_rel_path TEXT NOT NULL,
  text TEXT,
  section TEXT NOT NULL DEFAULT '',   -- '' = no heading

  PRIMARY KEY (from_document_id, position),
  FOREIGN KEY (from_document_id) REFERENCES documents(id) ON DELETE CASCADE,
  UNIQUE (from_document_id, to_source, to_rel_path, section)
);
CREATE INDEX IF NOT EXISTS document_links_target ON document_links (to_source, to_rel_path);
```

- The target is stored as `(to_source, to_rel_path)`, not as a document ID. Thus the ingest order does not matter, and a re-ingest of the target does not break the edge.
- The connection already sets `_foreign_keys=ON`, so the cascade works.
- An old DB gets the empty table at start. Links come only with a re-ingest (`ingest-clean`).

## Query-time resolution

One new port in the retrieval step, for example `LinkLoader.LinksOfDocuments(ctx, docIDs, filter)`. The SQLite adapter runs two queries, out and in. Both apply the request filter to the **other** document of the edge:

```sql
-- outgoing: the targets of the result documents
SELECT l.from_document_id, l.section, l.text, t.id, t.name, t.metadata_json,
       (SELECT c.id FROM chunks AS c WHERE c.document_id = t.id ORDER BY c.position, c.id LIMIT 1)
FROM document_links AS l
JOIN documents AS t ON t.source_name = l.to_source AND t.rel_path = l.to_rel_path
WHERE l.from_document_id IN (...) <filter on t>
ORDER BY l.from_document_id, l.position;

-- incoming: the documents that link to the result documents
SELECT t.id, l.section, l.text, f.id, f.name, f.metadata_json, (first chunk of f)
FROM documents AS t
JOIN document_links AS l ON l.to_source = t.source_name AND l.to_rel_path = t.rel_path
JOIN documents AS f ON f.id = l.from_document_id
WHERE t.id IN (...) <filter on f>
ORDER BY t.id;  -- the adapter sorts links_in by the shown title
```

- **The identity of a target is `source + rel_path`**, the same as the document identity ([0002](../decisions/0002-document-identity-derived-db.md)). Titles are not unique in each source, and the links of file-based sources are paths. The exporter maps other link forms (a wiki title, a URL) to the `rel_path`.
- **The filter applies to the other document.** A link never shows a document that the same request cannot `read`. Later, views use the same rule as an access boundary.
- A target without chunks (no first chunk) is not shown.
- Cost: max. 10 result documents, two indexed queries for each `rag` call. The first-chunk subquery needs the index `chunks(document_id, position)`. Without it, each link row scans all chunks. The index comes from the read-side DB optimization batch.
- The SQL of the filter (`filterClause`) uses the fixed table alias `d`. In both queries, the other document has this alias.

## Output of the `rag` tool (shape b)

The result changes from a JSON array of chunks to one object. A new step function `AttachLinks(ctx, chunks, filter, deps) (SearchResult, error)` in the retrieval step package builds it. It loads the links through the `LinkLoader` port and applies all rules of this section (grouping, title prefix, limits, small documents). The callers (`rag` MCP handler, `retrieve` CLI) call `RunRetrieval` and then `AttachLinks`, in the same way as the ingest calls extract, chunk and embed. `RunRetrieval` does not change, and the eval calls only `RunRetrieval`, so the eval does not change.

```json
{
  "chunks": [
    {"id": 4711, "doc_id": 12, "rank": 1, "doc_title": "Process list.md", "position": 23, "score": 0.03,
     "breadcrumb": "Administration", "text": "...", "source_weight": 0.8, "source_name": "wiki"}
  ],
  "documents": {
    "12": {
      "title": "Process list",
      "source": "wiki",
      "metadata": {"doc.url": "...", "doc.freshness": "current"},
      "links_out": [
        {"section": "Administration", "total": 11, "chunk_id": 4711, "links": [
          {"title": "Leave request", "chunk_id": 901}
        ]},
        {"section": "Software development", "total": 7, "chunk_id": 4702}
      ],
      "links_out_total": 33,
      "links_in": [
        {"title": "Main page", "section": "Processes", "chunk_id": 17, "text": "process list"}
      ],
      "links_in_total": 1
    }
  }
}
```

| Rule | Value |
|---|---|
| field names | snake_case JSON tags for all output fields, also the existing chunk fields (`ID` → `id`, `DocTitle` → `doc_title` and so on). The output shape changes in stage 1 anyway, so all clients get one break, not two. |
| `chunks` | the chunks in rank order, each with the new field `doc_id`, without `metadata` |
| `documents` | a map: one entry for each document of the chunks, key = document ID as a string. The model goes from `doc_id` of a chunk directly to its document. `metadata` moves from each chunk to here, so it does not repeat. |
| `links_out` | one group for each `section`, in the order of the first link of each section. The links without a section form the group `""`. |
| chunk section | the chunk breadcrumb without the title: if the breadcrumb starts with `<doc.title> > `, remove this prefix. If the breadcrumb equals `doc.title`, the section is `""`. docling puts the title into the breadcrumb only for some heading structures (wiki DB, 2026-10-21: in one hub page the breadcrumb is `Administration`, in another one it is `<title> > Review needed`), so the prefix is optional. |
| open group | A group is open (it has `links`) when its section **equals** the section of a result chunk of this document. No prefix rule: a chunk contains only the text of its own section, so a hit in `Internal docs > Review needed` opens only that group, not `Internal docs`. With more than one chunk of the document in the result, more groups can be open. |
| closed group | only `section`, `total` and `chunk_id`. The model sees all groups of a hub in a few lines. |
| `chunk_id` of a group | the first chunk of the document (in `position` order) whose chunk section equals the group section. The model calls `read {chunk_id, mode: section}` and gets the full list as text. No match: no `chunk_id`. |
| small document | If `links_out_total` ≤ 10, all groups are open. The section rule applies only to larger documents (hubs). Most documents have 1–5 links, and closed groups would hide their titles. |
| no section match | If no result chunk of a larger document matches a section, all groups are closed. |
| closed group, known limit | `read {mode: section}` with the `chunk_id` of a closed group returns the section text. The text contains the link titles, but no `chunk_id` of the targets. The model then needs `rag` with the title: 2 calls in place of 1. This happens only on hubs. A later option is a `read` mode for links. |
| `links_in` | flat list, sorted by the shown title, each entry with the `section` in the linking document. Not grouped. An incoming link from `<hub> > Review needed` tells the model that a person marked this page for review. |
| `title` of a link | `doc.title` from the metadata of the linked document if it is a string, else the document name |
| `chunk_id` of a link | the first chunk of the linked document, in `position` order. The model calls `read {chunk_id, mode: document}` or `section`. |
| `text` | only when the edge has one |
| limits | constants in the step: max. 50 links in one open group, max. 50 entries in `links_in`, 10 for the small-document rule. They are safety limits, not a selection, and not config values. `*_total` gives the real count. |
| no links | empty lists and totals 0. The keys stay, so the shape is always the same. |


**Tool description:** add two sentences to `rag`: "Each document lists its linked documents, grouped by the section that contains the link. Open a document or a closed group with read and its chunk_id." The `read` description gets no change.

**Query log:** each `rag` line gets the number of returned links. It holds no link titles.

## Other surfaces

| Surface | Change |
|---|---|
| `read` result | Stage 1: none. A later option: `links_out` and `links_in` in the `document` mode. |
| `catalog` | none, because `links` is not in `metadata_json` |
| `retrieve` CLI | the same object as the MCP tool, as one JSON document. This also fixes the "output is not JSON" backlog row. |
| eval | none. The eval uses `RunRetrieval`. Acceptance: a full re-ingest of the dev corpus gives the same metrics as the reference run. |

## Not in the scope

- Anchor text in the search (stage 2), expansion and in-degree (stage 3)
- links across sources
- link targets by title or URL. The exporter resolves them to `rel_path`.
- links in the middle of a chunk (which link belongs to which chunk)
- a check for other array values in the sidecar

## Acceptance

1. A sidecar with `links` gives rows in `document_links` in the sidecar order. Self links and duplicates are dropped.
2. A wrong shape or an unknown field fails only that document (invalid sidecar). A rule error (`to` with `..`, more than 1000 entries, no `doc.title`) is an `ErrInvalidMetadata` with a clear message.
3. `links` in the source attributes gives a config error at start.
4. `metadata_json` and the `catalog` output contain no `links` key.
5. The `rag` result has the object shape. Each document appears one time with its links. A target that does not exist is not shown. Only the groups of the result chunk sections are open.
6. With a `sources` or `metadata` filter, no link points to a document outside the filter.
7. `read {chunk_id}` with the `chunk_id` of a link gives the target document.
8. Full re-ingest of the dev corpus (no sidecars with links) + `just eval`: the same metrics as the reference run after the chunker order and table fixes of 2026-10-22.
10. A sidecar with `links` and without a string `doc.title` rejects the document. A chunk breadcrumb with and without the `<doc.title> > ` prefix gives the same chunk section.
11. The `rag` output and the `retrieve` output use snake_case field names.
9. By hand: wiki test export. A hit in a section of the process list hub shows that group open and the 7 other groups closed, each with a `chunk_id` that `read {mode: section}` opens. `links_in` of a page that a hub lists under a review heading shows that section. `links_in` of a process page contains the process list hub.

## Decisions at the lock (2026-10-22)

| Point | Decision |
|---|---|
| Hub problem | Group by `section`, open only the group of the hit chunk. No cut by list order. |
| Status words in headings (for example "obsolete", "review needed") | rag-cli and the exporter do not interpret them. The section text goes to the model, and the model interprets it. A word list in the exporter is brittle and specific to one source. |
| Section match | `doc.title` required with `links`, title prefix removed, then an exact match |
| Target identity | `source + rel_path`, not `doc.title` |
| `documents` | a map with the document ID as the key |
| Field names | snake_case for all output fields, now |
| `links_in` order | by the shown title |
| Breadcrumbs | checked in the wiki DB on 2026-10-21: the sections of the 3 hubs are correct, also nested sections. The test export moves all page headings one level below `# <title>`. |

## Changes in version 2 (2026-10-22, review before the build)

| Change | Reason |
|---|---|
| `kind` removed (field, column, unique key, output) | No stage 1 function uses it. Unknown fields are rejected, so a later `kind` is an addition without a break. |
| Step function `AttachLinks` after `RunRetrieval`, called by both callers | The logic is in one place in the step package and testable without MCP or SQLite. No step wraps another step. |
| Small-document rule (≤ 10 links: all groups open) | Without it, a typical document shows closed groups and no titles. |
| Limits as constants, not config values | Safety limits, not tuning values. |
| Index `chunks(document_id, position)` | `chunks` has no index on `document_id`. `EXPLAIN QUERY PLAN` shows a full scan for each first-chunk subquery. |
| Closed group: known limit written down | `read` of a group gives text, not target IDs. Accepted for hubs. |

## Build status (2026-10-22, branch `graph_stage_1`)

| Acceptance | Evidence |
|---|---|
| 1, 2, 10 | `internal/extract/step/links_test.go`, `extract_test.go` (`TestExtractMetadata`), `internal/platform/extraction/source/source_doc_provider_test.go` (`TestSourceDocLinks`), `internal/platform/sqlite/extraction/extracted_doc_sink_test.go` (`TestSaveExtractedDocLinks`) |
| 3 | `internal/platform/config/config.go` (check in `validate`). No test: the config package has no tests. |
| 4 | the provider removes `links` from the metadata map (`TestSourceDocLinks`) |
| 5, 6, 11 | `internal/retrieval/step/links_test.go` (`TestAttachLinks`, `TestChunkSection`), `internal/platform/sqlite/retrieval/link_loader_test.go` (`TestLoadLinks`) |
| 7 | e2e in Cursor (GPT 5.6, argon4, Opus), query log `data/query.log` of 2026-10-22: 16 of 20 read targets of the first runs are chunk position 0, which is the `chunk_id` of a link. |
| 8 | `AttachLinks` runs after `RunRetrieval`, so the links cannot change the rank. Run `20261022T192853Z` (new wiki export + chunker fixes of `07070d9`) is the new reference run. |
| 9 | Partly. By hand: a hit on the process list hub in a section without links shows all 8 groups closed, each with a `chunk_id`. In the e2e run, the model opened a closed group of the hub with `read {mode: section}`. Not seen: a hit in a link section with only that group open, and `links_in` under a review heading. |

An end-to-end run with 4 small test documents (scratch DB) gave the expected `document_links` rows and `retrieve` output: a missing target is not shown, and a breadcrumb without the title prefix matches its section.
