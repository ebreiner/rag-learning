---
type: requirement
title: Exporter contract
status: accepted
date: 2026-10-21
tags: [exporter, sidecar, source, metadata, contract]
---

# Exporter contract

What rag-cli expects from each exporter. An exporter is each program that writes the documents of a source folder. How an exporter gets its data is not part of this contract.

Sources: handover §3 ([rag-cli-handover](rag-cli-handover.md)), [0002](../decisions/0002-document-identity-derived-db.md), the [interpretation](rag-cli-handover-interpretation.md), [sidecar-metadata](../designs/sidecar-metadata.md), [retrieval-filters](../designs/retrieval-filters.md).

## Folder

- One folder for each source. `config.yaml` gives its path, its name and its `attributes` (`sources[]`).
- The exporter is the only writer of the folder. rag-cli only reads it.
- rag-cli reads the full tree. It skips folders whose name starts with `.`.
- The file extension selects the documents: `allowedExtensions` in `internal/platform/extraction/source/source_doc_provider.go`. These are PDF, Word, PowerPoint, OpenDocument text, Markdown and plain text, HTML, AsciiDoc and LaTeX. Spreadsheets (`.xlsx`, `.csv`) are not in the list. Other files give a warning and are skipped.
- The document identity is the source name plus the path relative to the source folder. Keep the paths stable between exports.
- Two files with the same content: the first file wins, and the second file and its metadata are lost.

## Document text

- docling converts each document. Convert formats that docling cannot read, for example wikitext, to Markdown or HTML.
- rag-cli builds the breadcrumb of each chunk from the headings. Give Markdown one top heading `# <title>`, and put all other headings below it.
- rag-cli does not split tables. A very large table becomes one very large chunk.

## Sidecar

- Name: the document file name plus `.meta.json`, in the same folder. Example: `process-list.md` → `process-list.md.meta.json`.
- Optional. Without a sidecar, the document gets the source `attributes` only.
- One flat JSON object. The keys contain dots as namespaces, for example `doc.title`. The values are bool, number or string.
- rag-cli merges the source `attributes` with the sidecar. The sidecar wins for each key.
- **Required:** `classification.personal_data` (bool), from the sidecar or from the source `attributes`. Without it, rag-cli rejects the document.
- An invalid sidecar (no valid JSON) rejects only its own document.
- rag-cli does not interpret other keys. It shows them in the result `Metadata` and in `catalog`, and the `metadata` filter can use them.

## Metadata values for filters

- A filter is equality only, and several keys combine with AND. There is no OR on values and no "older than".
- Thus give a value that a model filters on as a bool or a small enum, not as a date.
- `catalog` shows max. 20 values for each key. A key with more values (title, URL, ID) is for the result only.

## Shared keys (convention)

All exporters use these keys with the same meaning. rag-cli does not check them.

| Key | Value |
|---|---|
| `doc.title` | a human-readable title |
| `doc.url` | a link that the user can open |
| `doc.last_updated` | `YYYY-MM-DD`, from the source system, never from the file time |
| `doc.freshness` | `current` (year ≥ 2024), `probably_stale` (2022–2023), `dangerous` (< 2022 or marked as old in the source) |
| `doc.source_id` | the ID of the document in the source system |

## Links (optional)

- The reserved sidecar key `links` holds the outgoing links of the document: a list of objects `{"to": "<rel_path>", "text": "<anchor text>", "section": "<heading path>"}`. `text` and `section` are optional. Other fields reject the document.
- `to` is the path of the target relative to the source folder, in the same source. Write only links to documents of the same export. A target that does not exist is not shown.
- `section` is the heading path of the link in the document, with ` > ` as the separator and without the title. rag-cli groups the links by it. Write the heading text only, and do not interpret it.
- A sidecar with `links` must have `doc.title` (non-empty string). The Markdown starts with `# <doc.title>`, with the same text.
- rag-cli computes the incoming links. `links` is not metadata: no filter and no catalog entry.
- Details: [document-links](../designs/document-links.md).

## Update

- rag-cli does not update or delete single documents. A changed file stops the ingest.
- Procedure: export the full source into an empty folder, then run a full re-ingest (`just ingest-clean`).
- A sidecar change takes effect only after a full re-ingest.
