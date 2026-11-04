---
type: requirement
title: Interpretation of exporter handover 2
status: open
date: 2026-11-03
tags: [exporter, handover, interpretation, contract, links, excel]
---

# Interpretation of exporter handover 2

- Source: [exporter-handover-2](exporter-handover-2.md) (from the exporter project, 2026-10-21, unchanged). This file is
  the interpretation of team rag-cli.
- The binding rules for exporters are in [exporter-contract](exporter-contract.md). Where the handover and the contract
  differ, the contract wins.
- The first handout is not in this repo.

## Comparison with the contract and the build state (2026-11-03, branch `graph_stage_1`)

| Handover point | State in rag-cli | Result |
|---|---|---|
| §1.1 `.xlsx` and `.csv` are not accepted | Correct. `allowedExtensions` in `internal/platform/extraction/source/source_doc_provider.go` | Agreed. The rag-cli decision (docling fallback or exporter conversion) is open in the [backlog](backlog.md). |
| §1.2 `doc.links_to` is out of date | Correct. The form is the reserved key `links`. | Agreed. |
| §2 identity, one top heading, sidecar, filter values, shared keys, full export | Same as the contract. | Agreed. |
| §2 heading shift (`= X =` → `## X`) | Only in the lock notes of [document-links](../designs/document-links.md). The contract has the general rule "one top heading". | Agreed. The contract can show it as an example. |
| §2 `links` "design draft, not built yet" | **Out of date.** Stage 1 is built on `graph_stage_1` ([document-links](../designs/document-links.md) version 2, locked 2026-10-22). | Exporters can write `links` now. |
| §2 `links` fields | The contract adds rules that the handover does not have: `doc.title` (non-empty string) is required with `links`, and the Markdown title must be the same text. Unknown fields reject the document. Max. 1000 entries. No `..` and no absolute path in `to`. | An exporter that follows only the handover can write rejected documents. Use the contract. |
| §3 tables and Excel | Exporter scope. The contract says: "rag-cli does not split tables". | The row-section form agrees with this rule, because it prevents very large table chunks. No rag-cli work. |
| §3.4 Text-to-SQL | A rag-cli topic. | Not in the backlog. Add it only when a user need appears. |

## Known gaps (not from the handover)

- RC18 requires three classification attributes. The contract requires only `classification.personal_data`
  ([sidecar-metadata](../designs/sidecar-metadata.md)).

## Open questions

| Question | For | Status |
|---|---|---|
| Add the heading-shift example to the contract? | User | Open. |
| Spreadsheets in rag-cli: docling fallback or exporter conversion only? | User | Open (backlog carry-over). |
