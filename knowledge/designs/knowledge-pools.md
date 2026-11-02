---
type: design
title: Knowledge pools with a doc store
status: draft
date: 2026-10-23
tags: [pools, doc-store, connectors, event-log, access, later]
---

# Knowledge pools with a doc store

Status: draft for later. The current work is the PoC in [0005-pool-poc](../experiments/0005-pool-poc.md). Its findings change this design before it can be locked.

## Problem

Today, each change to the content of a rag-cli DB needs the tech owner: new exporters, new folders, a new ingest. The people with the domain knowledge must wait. The phase is learning, not product, so slow changes stop the learning.

## Three jobs

Fetch, select and index are separate jobs with separate owners:

```
sources ──connector──▶ doc store ──pool query──▶ pool ──rag-cli──▶ MCP
(Jira, wiki, SMB,      (raw docs,                (doc set at
 Joplin, …)             sidecars, events)         a log position)
```

## Requirements

| # | Requirement | Why |
|---|---|---|
| R1 | Domain experts define and change the content of a pool without a code change. | This removes the bottleneck. |
| R2 | The tech owner (DevOps) owns the connectors, the doc store, access, rag-cli and the instances. The pool owner owns the pool query, the golden set and the decision to publish. | Each part stays with the people who can operate it. |
| R3 | A new source needs only a new connector. Pools and rag-cli do not change. | The doc store is the interface. |
| R4 | Each document has a stable ID `<source>/<native id>`. Each connector defines its scheme as a fixed part of its contract. | Golden sets, links and the audit refer to it. |
| R5 | Each build records all its inputs: pool query, doc store position, extractor version and config, embedding model, rag-cli commit, golden hash. | Builds can be reproduced and compared. |
| R6 | Each build gets an eval against its golden set, compared with the last published build. | Experts must see the effect of a change. |
| R7 | The store keeps the raw documents. A new extraction needs no new fetch from the source. | Docling is a risk as a dependency. |
| R8 | One extraction is done once for each set of raw content, extractor version and config. | Extraction is the cost driver. |
| R9 | rag-cli gets the full extraction structure (docling JSON), not flat Markdown. | Markdown loses the signal. |
| R10 | Each document has the access tag of its source from day 0. Enforcement can come later. | The tag cannot be added to old data later. |
| R11 | The audit answers "which pools and builds contained document X". | DevOps needs it. |
| R12 | A delete in the source goes into the store. Old builds stay explainable. | Data hygiene and reproducibility. |
| R13 | rag-cli stays a tool for one pool. Its input is a folder. | Keep the spirit of the tool. The PoC breaks this rule on purpose. |
| R14 | The first implementation is the simplest possible, but the model allows a scale-out later without a redesign. | Learn now, no dead end. |

## Model: event log and derived views

| Concept | Meaning | Simplest implementation |
|---|---|---|
| Log | Connectors write `upsert(doc_id, raw_hash, sidecar, access)` and `delete(doc_id)` | One SQLite table `events(seq PRIMARY KEY, …)` |
| Blobs | Raw files by content hash | Folder `blobs/sha256/ab/…` |
| Docs at position N | Last event for each `doc_id` with `seq ≤ N`, without deletes | SQL view with a window function |
| Extract | f(raw blob, extractor, version, config) | Eager extraction, docling frozen during the learning phase |
| Pool | Pool query on the docs view at position N | Resolve, write the folder, ingest, eval |

- A snapshot is a log position. The same position and the same query give the same document set.
- Sources such as Jira, a wiki or SMB do not send events. A connector pulls, compares and writes events. A webhook can write the same events later, and nothing downstream changes.
- The queue is the table. A consumer keeps its last `seq`.
- The log is the audit trail (R11, R12). Do not remove events during the learning phase.
- Pool queries and build records are normal rows with a version, not events.
- Each event records the connector version, because sidecar schemas change.

## Pool query

- Read-only SQL, one statement, returns document IDs.
- Each connector supplies typed views, for example `jira_issues(doc_id, project, status_category, …)`. They are the part of the connector contract that experts see.
- No wall-clock functions. The snapshot time is a parameter `:as_of`.
- The query runs only on the documents that the author may read.

## Access

- Build side: the build tool reads the LDAP groups of the author and filters the views.
- Consume side, without OIDC: one pool = one MCP endpoint = one tool in OpenWebUI, restricted to an OpenWebUI group from LDAP. A pool contains one access level only.
- Per-user filtering inside one pool needs the user identity in the MCP server (OIDC, maybe RAR, RFC 9396). Not before OIDC exists.

## Decided

- Git is not the doc store.
- Extraction is eager. 5–10k files at most are acceptable with spare compute.
- Docling stays frozen in the learning phase. Extractor versions and configs are a later topic.
- No extract cache in front of docling for now.

## Open

- Typed views for each connector: do experts see only those, or also the raw tables?
- Pool lifecycle (build, eval, publish, serve) and the publish gate.
- How eval and golden sets for each pool work with document IDs from R4.
