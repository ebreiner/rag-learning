---
type: design
title: Read tool (RC6)
status: locked
date: 2026-10-19
tags: [read, rc6, mcp, renderer, filter, outline]
---

# Read tool (RC6)

Implements RC6 of the [handover](../requirements/rag-cli-handover.md): from a search hit to the chunks around it, to its section, or to the full document. It is one of the tools of [0004](../decisions/0004-mcp-tool-surface.md). It uses the filter of [retrieval-filters](retrieval-filters.md).

## Goal

The model goes from one `rag` hit to more context with one call. A large document never gives more text than a configured limit.

## Facts (dev corpus, 2026-10-19)

| | Average | Maximum |
|---|---|---|
| Chunk | 632 characters | 5,122 characters |
| Chunks for each document | 5.4 | 62 |
| Document | 3,389 characters | 26,275 characters |

Production instances will hold much larger documents, for example contracts and regulations. Thus the limit and the outline are necessary, although no current document needs them.

## Tool

One MCP tool `read`:

| Parameter | Type | Note |
|---|---|---|
| `chunk_id` | number, required | the `ID` of a `rag` hit. The document and the section follow from this chunk. |
| `mode` | enum, required | `neighbours`, `section`, `document`, `page` |
| `n` | number, optional | only for `neighbours`: chunks before and after. Default 2, maximum 5. |
| `sources`, `metadata` | optional | the same filter as `rag`. If the document of the chunk does not match, the tool returns an error. |

A CLI command `read --chunk <id> --mode <mode> [--n] [--source] [--meta]` gives the same result as JSON, for tests by hand.

## Modes

| Mode | Text |
|---|---|
| `neighbours` | the chunk and `n` chunks before and after it in the same document, by `position` |
| `section` | all chunks of the document whose breadcrumb is the breadcrumb of the chunk, or starts with it plus ` > ` |
| `document` | all chunks of the document in `position` order |
| `page` | the chunks of the document from the given chunk on, in `position` order, up to the limit. It is the continuation of the page fallback. |

## Limit and fallback

- `read.max_chars` in the config, required, > 0. Suggested value: 30000 characters, about 7–8k tokens.
- If the text of `neighbours`, `section` or `document` is not larger than the limit, the tool returns the text.
- Else the fallback chain applies:
  1. **Outline:** the distinct breadcrumbs of the document or section in `position` order. Each entry has the breadcrumb, its character count, and the `chunk_id` of its first chunk. The model then calls `section` with that `chunk_id`. A section that is too big gives the outline of its sub-sections. The outline applies only when it has at least 2 entries and its JSON fits the limit.
  2. **Page:** the chunks from the start of the document or section up to the limit, plus `next_chunk_id`. The model continues with `mode: page` and that `chunk_id`.
  3. **Warning:** if one chunk alone is larger than the limit, no page fits. The tool returns a warning that the document has too little structure to read in parts. The model then uses `rag` for that document.

## Text (variant b)

- The chunk text starts with `breadcrumb + "\n\n"` (chunker, content chunks). `read` removes this prefix when it is present.
- `read` writes a breadcrumb once as a heading line when it changes, then the chunk texts.
- Lists and tables keep the form that the chunker gives them.
- A renderer from the extraction nodes (old T15) is not in this scope. If it becomes necessary, the first step is one shared render function in the chunk step, not a second renderer.

## Result

```json
{
  "document": "file name",
  "source": "manual",
  "metadata": {"classification.personal_data": false},
  "mode": "section",
  "chars": 4210,
  "text": "…",
  "too_big": false,
  "outline": null,
  "next_chunk_id": null,
  "warning": null
}
```

Only the fields of the result type are set: `text`, or `outline`, or `text` plus `next_chunk_id` for a page, or `warning`.

## Parts

| Part | Change |
|---|---|
| Domain | A read step: select the chunks for the mode, apply the limit and the fallback chain, build the outline, join the text. Pure functions with table tests. |
| Port | One port that loads the document of a chunk, with the filter: document name, source, metadata, and all its chunks (`id`, `position`, `breadcrumb`, `text`). A document of 70k words is a few hundred chunks, so one load is acceptable. |
| Adapter | SQLite: one query for the chunk and its document with `filterClause`, one query for the chunks of the document by `position`. |
| Config | `read.max_chars` |
| MCP, CLI | the tool `read`, the command `read` |
| Telemetry | span attributes: chunk ID, mode, result type, characters |

## Not in the scope

- The renderer from the extraction nodes (old T15).
- A document name as input. The input is always a chunk ID.
- The `rag` result with more context. `rag` stays as it is.

## Acceptance

1. Table tests for each mode: neighbours at the start and the end of a document, section with sub-sections, document.
2. Limit tests: outline, outline of a section, page with `next_chunk_id`, the last page, warning.
3. Text: the prefix is removed, each breadcrumb appears once.
4. Filter: a chunk of an excluded document gives an error, a matching chunk gives the text.
5. Eval does not change (no change to `rag`).
6. By hand on the dev corpus: `read --mode section` for chunk 679 gives the section "Schwellwertüberwachung anlegen". With a small `max_chars`, the same call gives an outline or a page.
