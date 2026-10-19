---
type: decision
title: A sources table replaces collections
status: accepted
date: 2026-10-19
tags: [source, schema, storage, extraction, retrieval]
---

# 0012 A sources table replaces collections

Adds to [0010](0010-source-single-term.md) and [0002](0002-document-identity-derived-db.md). Supersedes these parts of [0010](0010-source-single-term.md): the column name `documents.source` and the rename of the walk package as a step of this change. Supersedes this part of [0001](0001-config-yaml-data-sources.md): "optional weight".

## Context

- The config file ([0011](0011-config-file-and-composition-root.md)) lists the sources with name, path and weight. The old `collections` table got its rows from CLI flags at each `extract` run.
- After the config refactor, the documents had no collection. The weight for each chunk was then 0, and hybrid retrieval sorted the results by chunk ID.
- [0002](0002-document-identity-derived-db.md) needs the relative path for the document identity.

## Options considered

1. Keep `collections` and fill it from the config. Two names for one thing stay in the schema.
2. A new `sources` table and a foreign key on each document. Accepted.
3. Synchronize the table with the config, with a delete of removed sources. Not necessary now, because the DB is derived and a full rebuild is permitted ([0002](0002-document-identity-derived-db.md)).

## Decision

- **Schema:**
  - Table `sources` with `name TEXT PRIMARY KEY` and `weight REAL NOT NULL`, with `CHECK(weight > 0)`.
  - `documents` gets `source_name` with a foreign key to `sources(name)`, and `rel_path`.
  - `UNIQUE(source_name, rel_path)` is on `documents`.
  - The `collections` table is removed.
- **Write:** before the extraction, the composition root writes all configured sources (`sqlite.SaveSources`) in one transaction. An existing source gets the weight from the config.
- **Weight:** the weight is required in the config and must be > 0. It is the only field of a source in the DB for now.
- **Duplicate content:** a document with a known content `sha256` is skipped. The first source wins. If the known document is in a different source, `ingest` writes a warning.
- **Relative path:** the walk adapter computes `rel_path` against the source path, and the extraction writes it.
- **Retrieval:** only the SQL changes. It joins `sources` in place of `collections`. The Go names `CollectionWeigher`, `collectionRerank`, `CollectionName` and `CollectionWeight` stay until the retrieval rework.

## Consequences

- Each document has a valid source, so the hybrid weight works again.
- An old DB does not work with the new schema. `CREATE TABLE IF NOT EXISTS` does not change existing tables. Delete the DB and run `ingest` again.
- The duplicate rule is different from [0002](0002-document-identity-derived-db.md), where the same content at two paths is two documents. For now the first document wins. A later change can give one document more than one source.
- The MCP output still has the fields `CollectionName` and `CollectionWeight`. The retrieval rework changes them to the source term of [0010](0010-source-single-term.md).
- Not in this change:
  - the sidecar metadata
  - the add, replace and delete logic of [0002](0002-document-identity-derived-db.md)
  - a change of the golden set to `rel_path`
  - the rename of the walk package
- Evidence: the same eval run as in [0011](0011-config-file-and-composition-root.md). It has the source weights 0.8 and 0.4, as the baseline had for its collections, and it has equal metrics.
