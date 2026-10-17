---
type: decision
title: Document identity by path, DB as a derived artifact
status: accepted
date: 2026-10-17
tags: [storage, schema, identity, re-ingestion]
---

# 0002 Document identity by path, DB as a derived artifact

Supersedes this part of [0000](0000-migrated-baseline.md): Extraction ("Dedup on content `sha256`", the duplicate warning for a different collection).

## Context

- [Handover](../requirements/rag-cli-handover.md) RC11: changed and deleted documents must be updated.
- Now the content `sha256` is the document identity. A changed file gets a new hash, so `extract` adds a second document. The old document and its chunks stay. Delete is not possible, because no key connects the old document to its file.
- The central directory is the source of truth. Exporters are the only writers (handover §3).
- 0000 has no migrations: all DDL runs as `CREATE ... IF NOT EXISTS`. Many schema changes come with the handover.

## Options considered

1. Keep the hash as the identity. Update and delete are then not possible.
2. Identity = (data source, relative path). The hash is only a change test. Accepted.
3. Identity from a sidecar ID field. Not all exporters can supply a stable ID.
4. Schema migrations for each change. Much work for a DB that can be built again from the directory.

## Decision

- Document identity is (data source, relative path).
- The content `sha256` only shows if a file changed.
- The DB is a derived artifact of the central directory and `config.yaml`. A schema change means a full re-ingest. There are no migrations.

## Consequences

- RC11 becomes possible: new path → add, changed hash → replace, missing path → delete.
- The same content at two paths is two documents. This is accepted.
- The bugs "second document for a changed file" and `GetLatestExtractionOfDoc` without a "latest" filter disappear with this change, if replace removes the old extraction.
- A full re-ingest costs docling and embed time. This is acceptable for the current corpus sizes.
