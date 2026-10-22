---
type: concept
title: SQLite query plans in the retrieval
status: current
date: 2026-10-22
tags: [sqlite, fts5, sqlite-vec, query-plan, performance, retrieval]
---

# SQLite query plans in the retrieval

Rules for the read-side SQL in `internal/platform/sqlite/retrieval/`. The code has short comments that link to this file.

Evidence: agent branch `worktree-agent-ad2a555f443cfd7c6`, commits `556cfd8` and `2237992`, harness `readbench_test.go` (build tag `readbench`, commits `61c2e60`, `e656780`, `fdb1bd8`). Measured on a copy of the dev DB (7700 chunks, 734 documents), median of 25 runs with 50 inputs each.

## Index `chunks(document_id, position)`

- Queries that read the chunks of one document in position order use it: the `read` tool (`document_loader.go`), the first chunk of a link target (`firstChunkOfD` in `link_loader.go`) and the section starts (`querySections`).
- Without it, each such query reads all chunks and sorts them in a temp B-tree. The rowid is the last key of each index entry, so `ORDER BY position, id` needs no sort.
- First chunk of all 734 documents: 588 ms → 5 ms. `read` document lookup: 46 ms → 5 ms for 50 calls. First chunk of all 570 link targets: 575 ms → 1 ms.
- `NewConn` runs `sqlc_schema.sql` at each start, so `CREATE INDEX IF NOT EXISTS` adds the index to an existing DB. No re-ingest.

## `CROSS JOIN` in the filtered FTS query

- SQLite runs a join as nested loops. The planner selects which table is the outer loop.
- The expensive step must be the outer loop, so that it runs one time. Here that is the FTS5 `MATCH`: it gives the matching chunk IDs, then `chunks` and `documents` are primary-key lookups.
- The index above made a second order look cheap: `documents` by `source_name`, then `chunks` by `document_id`, then `chunks_fts` for each chunk. FTS5 cannot test one rowid cheaply, so the `MATCH` runs one time for each chunk. FTS with a source filter: 46 ms → 17878 ms.
- The planner has no statistics (no `ANALYZE`), and FTS5 as a virtual table gives only rough costs. Thus it selects the bad order.
- In SQLite, the left table of a `CROSS JOIN` always stays the outer loop. `CROSS JOIN` gives the same rows as `JOIN`. It fixes the order.
- Rule: after a new index, check `EXPLAIN QUERY PLAN` of each query that joins `chunks` and `documents` with a filter.

## Filtered ANN: `chunk_id IN` inside the vec0 KNN

- A vec0 KNN query (`embedding MATCH ? AND k = ?`) applies a `WHERE` on joined tables only after it selects the k nearest chunks. A join thus gives "top k, then filter", which can return fewer than k chunks or none.
- The old query computed `vec_distance_l2` row by row on all filtered chunks, then sorted. It was exact, but slow.
- The current query gives the filtered chunk IDs to the KNN as `e.chunk_id IN (subquery)`. SQLite 3.38+ hands the full IN list to the virtual table, and vec0 applies it before the top k. The search stays exact: the k nearest chunks inside the filter.
- vec0 accepts only `ORDER BY distance`. For equal distances it gives the higher `chunk_id` first, the old scan the lower first. The KNN is thus a `MATERIALIZED` CTE, and the outer query sorts on `distance, chunk_id`.
- Same results as the old query (IDs, order, distances) for 50 source-filter and 50 metadata-filter queries.

| Hybrid, for each call (7.7k chunks) | Old: median / p95 | Now: median / p95 |
|---|---|---|
| `sources` filter | 229 / 1404 ms | 8 / 14 ms |
| `metadata` filter | 622 / 717 ms | 11 / 13 ms |

Known limits:
- If a group of exactly equal distances crosses the k-th place, the old and the new query can keep different chunks of that group.
- The unfiltered ANN still gives the higher `chunk_id` first in a tie. The filtered ANN gives the lower first.
- The query depends on vec0 support for `IN` on `chunk_id` in a KNN. Tests in `filter_test.go` (`TestFilterANN`) guard the empty filter and the tie order.
