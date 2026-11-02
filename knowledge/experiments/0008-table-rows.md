---
type: experiment
title: Table rows as self-contained lines, large tables split by rows
status: open
date: 2026-10-25
tags: [chunking, tables, eval]
---

# 0008 Table rows as self-contained lines, large tables split by rows

Backlog row "Final table treatment" in [backlog](../requirements/backlog.md). Branch `claude_experimental`, code in `internal/chunk/step/chunk.go` (`tableCandidates`).

## Setup

- Before: one table = one chunk, pipe rows (`a | b`) under one header line, caption and footnote at the end, no breadcrumb in the chunk text. A header cell in a lower row moved the data start below that row, so the code lost rows.
- After:
  - Header rows are the top rows without a data cell that has text. The header levels of a column are joined (`Register / Lo`).
  - Table with a header: each row is one line `Header: value; Header: value`. Empty cells are not written. A header cell in a lower row (a row header) is a value of its row.
  - Table without a header (also: only header rows, or a header over empty cells): pipe rows, no empty header line.
  - The cells set the table size when docling gives smaller `Rows` or `Cols` (docling gives `Cols 0` for some tables with cells).
  - Rows are packed in table order into parts of at most 1000 characters (breadcrumb included). Each part starts with the caption. The footnote is in the last part. The parts have consecutive positions, so `read` gives the neighbour parts of a hit.
  - Table chunks start with `breadcrumb\n\n`, the same as content chunks.
- Eval: `just eval` with `--baseline` run `20261022T192853Z`, same golden set. No golden query has its answer in a table, so the eval shows only side effects.

## Result

| Run | DB | Table chunks | Table nodes without chunk | hybrid doc r@1 | doc r@5 | doc MRR | chunk r@10 | chunk MRR |
|---|---|---|---|---|---|---|---|---|
| `20261022T192853Z` (baseline) | old chunker | 524 | not measured | 0.697 | 0.879 | 0.739 | 0.697 | 0.489 |
| `20261025T094011Z` | first version, `ingest-clean` | 418 | 209 of 525 | 0.667 | 0.848 | 0.723 | 0.727 | 0.494 |
| `20261025T094828Z` | final version, rechunk of a DB copy | 607 | 21 of 525 | 0.667 | 0.848 | 0.723 | 0.727 | 0.494 |

- Of the 21 table nodes without a chunk, 20 have no text. One has text; probably the walk does not reach it (a table inside a list or group).
- Doc level, hybrid: g23 doc rank 1 → 2, g22 5 → 6. Chunk level: g23 now a hit at rank 6 (before in the pool at 12).
- The first version lost 209 tables: 155 tables with one row of header cells only, about 50 tables with a header cell in a lower row or a header over empty data cells. The final version keeps them.

## Findings

- The text form does not change the rank for the current golden set. The two doc changes are one rank each.
- Wiki tables are mostly small (average about 230 characters). Only one table was larger than 8000 characters before the split.
- Many wiki tables come from templates: a header row over empty cells, or key/value tables with a row header in the first column. A rule that reads only the column header flag loses their text.
- `ingest --rechunk` fails on a DB with embeddings ("Error opening vector blob"). See the backlog row. The rechunk test ran on a DB copy after a drop of the vec0 table.

## Recommendation

Keep the change. Add 3–5 golden queries whose answer is one table row, in a separate golden run, and measure again.
