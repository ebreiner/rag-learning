---
type: requirement
title: Exporter handover 2 (rag-cli contract changes, tables and Excel)
status: locked
date: 2026-10-21
tags: [exporter, handover, contract, tables, excel, links]
---

<!-- Input from the exporter project. Body unchanged. Interpretation: exporter-handover-2-interpretation.md -->

# Handover 2: rag-cli contract changes + tables and Excel (2026-10-21)

  This handover adds to the first handout. Where the two differ, this one wins.
  Rules do not change: discuss first, build only after an explicit grant ("build it", "grant"). "Fine" or "agreed" is not a grant.

  ## 1. Corrections to the first handout

  1. **rag-cli does NOT accept `.xlsx` or `.csv`.** They are not in its list of extensions, so the ingest skips them with a warning.
     - The first handout said "xlsx is copied as it is". That is wrong. A copied xlsx never reaches the index.
     - Until the user decides otherwise, **exporter B must convert each spreadsheet to Markdown.** See section 3.
     - Accepted extensions: PDF, Word, PowerPoint, OpenDocument text, `.md`/`.txt`, HTML, AsciiDoc, LaTeX.
  2. **Handout item 5 (graph edges as `doc.links_to` arrays) is out of date.** The new form is in section 2. Do not build links before the user says so.

  ## 2. The rag-cli exporter contract (accepted)

  Rules that each exporter must follow:
  - **Identity** = source name + path relative to the source folder. Keep the paths stable between exports.
  - **One top heading.** Each Markdown file starts with `# <title>`, and all other headings are below it. rag-cli builds the breadcrumb of each chunk from the headings, for example `Rechtskataster > BImSchG §5`. A second `#` heading breaks the breadcrumb.
    - Example: wikitext `= X =` becomes `# X` in pandoc. Move all page headings one level down (`# X` → `## X`), and leave code fences unchanged.
  - **Sidecar** `<file>.meta.json`: one flat object, values bool/number/string. `classification.personal_data` is required (sidecar or source attributes).
  - **Filter values:** equality and AND only. Use a bool or a small enum, never a date. The catalog shows max. 20 values for each key.
  - **Shared keys:** `doc.title`, `doc.url`, `doc.last_updated` (YYYY-MM-DD, from the source system, not the file time), `doc.freshness` (`current` ≥2024, `probably_stale` 2022–23, `dangerous` <2022), `doc.source_id`.
  - **Update:** export the full source into an empty folder, then run a full re-ingest. rag-cli has no update of single documents.

  **Coming in rag-cli (design draft, not built yet):** the reserved sidecar key `links`, a list of objects:
  ```json
  "links": [{"to": "<rel_path in the same source>", "text": "<anchor text>", "section": "<heading path of the link, ' > ' separated, without the title>"}]
  ```
  - rag-cli shows the links grouped by `section` and computes the incoming links itself.
  - The exporter writes only the outgoing links to files in the same export.
  - The exporter does not interpret the meaning of headings. It writes the structure only, and the LLM interprets it.
  - For exporter A: the MediaWiki wikitext has `[[Target|text]]` and the heading lines. A Python prototype exists at home.
  - Do not build this before the user asks.

  ## 3. Tables and Excel: research results

  Sources are marked: [code] = source read, [docs], [paper], [opinion].

  ### 3.1 Strategy for each table shape

  | Shape | QMS example | Markdown form for the exporter |
  |---|---|---|
  | Record table (header + rows) | Rechtskataster, maintenance plan, risk analysis | **One row = one section.** `## <key column value>`, then one `- Header: value` line for each non-empty cell. Same idea as RAGFlow "Table" mode [code] and TARGET row-level form [paper]. |
  | Matrix / cross table | RACI, communication matrix, responsibility matrices | **Unpivot.** One line for each non-empty cell: `- <column label>: <value>`, grouped under `## <row label>`. docling uses the same triplet idea `row, column = value` [code]. Drop empty cells. |
  | Key-value block | header block of the QM plan | `- Key: value` lines, plain text, not a table [opinion]. |
  | Header on 2 rows / merged group headers | many QMS sheets | Join them as `Top / Sub`. No tool does this automatically [code]. |
  | Layout or form sheet | form-like sheets | Plain text in reading order [opinion]. |

  - **Sheet structure:** `# <file title>`, `## <sheet name>` when the workbook has more than one visible sheet, then the row sections one level below.
  - The breadcrumb then is `<file> > <sheet> > <row key>`.
  - BM25 needs these titles: on tables without titles, BM25 recall fell from 0.967 to 0.592 (TARGET, arXiv 2505.11545) [paper].

  ### 3.2 Resolve the workbook before conversion (Go: `github.com/xuri/excelize/v2`) [docs]

| Need | API | Rule |
|---|---|---|
| Sheets | `GetSheetList`, `GetSheetVisible` | Skip hidden sheets. Hidden rows and columns: `GetRowVisible` / `GetColVisible`. |
| Values | `GetRows` / `GetCellValue` | They give the cached, formatted value. Do not use formulas. A file without cached values gives empty cells, so log them. |
| Merged cells | `GetMergeCells` | Copy the value into the full range before you read the rows. |
| Table ranges | `GetTables` | Use the tables that the author defined when they exist. Use a heuristic only when they do not exist. |
| Comments | `GetComments` | Add them as a `- Note: …` line in the row section. |
| Dropdowns | `GetDataValidations` | The allowed values of a column. Useful as an enum for a sidecar key. |
| Colour | `GetCellStyle` + `GetStyle` | Use colour only where a file has a known meaning (for example red = overdue). Write it as text (`- Status: rot`). It does not generalize [opinion]. |

### 3.3 Config, because the shapes differ for each file

Heuristics are not reliable enough. Let the QMS config YAML say how to read each spreadsheet. Proposal for discussion:
```yaml
include:
  - path: "4.2 Rechtskataster.xlsx"
    sheets:
      - name: "Rechtskataster"     # default: all visible sheets
        shape: records             # records | matrix | text
        header_rows: 1             # 2 = join as "Top / Sub"
        key_column: "Gesetz"       # the heading of each row section
```
The dry run must show, for each sheet: the shape, the detected headers and the first 2 converted rows. Then the user can tune the YAML.

### 3.4 Not for now

- **docling with the raw xlsx:** it uses row 0 as the header, with no row headers [code]. Use it only as a fallback, and only if rag-cli accepts xlsx some day.
- **LLM summaries of tables:** they retrieved much worse than the real rows (TARGET: R@10 0.436 vs 0.741) [paper]. If ever used, add them as an extra chunk, not as a replacement.
- **Text-to-SQL** (sheet → SQL table + query tool): this is the correct answer for "how many" or "all items due before X". It is a rag-cli topic, not an exporter topic.

## 4. Next steps at work

1. The user sends the header rows of `ISO9001_Prozessübersicht.xlsx`, `6.2 QM-Plan.xlsx` and `4.2 Rechtskataster.xlsx`. Also ask: how many sheets, merged cells, 2-row headers, defined Excel tables, a key column?
2. Classify each main sheet as `records`, `matrix` or `text`. Propose the YAML entries.
3. Discuss the config proposal (3.3) with the user. Then get the build grant for B with the xlsx → Markdown conversion.
4. docx and pdf in B stay as before (copied as they are). rag-cli accepts them.
