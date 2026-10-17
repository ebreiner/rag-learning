---
type: decision
title: One ingest command for all data sources
status: accepted
date: 2026-10-17
tags: [pipeline, ingest, re-ingestion, cli]
---

# 0006 One ingest command for all data sources

Supersedes this part of [0000](0000-migrated-baseline.md): Deferred ("Pipeline mode: parked").

## Context

- [Handover](../requirements/rag-cli-handover.md) §3 shows `rag-cli ingest`. RC10 has many data sources. RC11 needs re-ingestion with a hash compare.
- Now `extract`, `chunk` and `embed` are separate commands. Each run needs flags.

## Options considered

1. Keep the separate commands and run them from a script for each data source.
2. One `ingest` command that reads `config.yaml` and runs extract, chunk and embed for all data sources. Accepted.

## Decision

- `ingest` reads `config.yaml` and processes each data source.
- For each file it compares the path and the hash with the DB ([0002](0002-document-identity-derived-db.md)): add, replace, delete or skip.
- The step commands stay for development for now.
- Scheduled runs and an exporter trigger (RC11, should) come later on top of `ingest`.

## Consequences

- The step functions stay. `ingest` is a new composition root.
- The chunk step needs tolerance for a failure of one document, like `RunExtract`.
