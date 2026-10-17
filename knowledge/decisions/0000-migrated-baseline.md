---
type: decision
title: Migrated baseline architecture
status: accepted
date: 2026-10-17
tags: [baseline, architecture, config, extraction, chunk, embedding, storage, collections, retrieval, mcp, eval, tests]
---

# 0000 Migrated baseline architecture

## Context

- Old project notes were in a separate memory store. Many notes were stale.
- This record replaces them. It records the architecture and the settled decisions that the code shows.
- Code state: branch `bugfix_bundle` = `origin/master` `9af6e91` (PR #25).
- When a note and the code disagree, this record follows the code.
- Open work is in [the backlog](../requirements/backlog.md).

## Options considered

1. One baseline record for all settled decisions. New records supersede parts of it later.
2. One record for each old decision. Many old decisions changed more than one time, so the history is noise.
3. No records. The code shows "what", but it does not show "why X and not Y".

## Decision

Use option 1. Each bullet gives the decision and, after "Reason", the rationale.

### Overview

- Go module `rag`, Go 1.26, Cobra CLI (`main.go` → `cmd/cli/root.go`). Commands: `extract`, `chunk`, `embed`, `retrieve`, `serve mcp`.
- Data flow: source files → docling-serve → `extraction_nodes` → `chunks` → vec0 tables + FTS5 → hybrid retrieval → MCP tool.
- Ports and adapters. Domain steps are in `internal/<stage>/step` with `ports.go`. Adapters are in `internal/platform/...`.
- The composition root is the `RunE` of each command. Steps get primitives or ports, never a config object. Reason: tests use fakes.
- The composition root opens one `*sql.DB` with `sqlite.NewConn(dbPath, rebuildFTSIndex)` and gives it to all adapters.
- Loop steps pull one unit (doc or batch) for each call and stop on `io.EOF`. The body of one unit is a separate function. Reason: `defer` and spans then close for each unit.
- Commands use `RunE` and return errors. Only `cli.Execute` calls `os.Exit(1)`. Reason: `os.Exit` skips deferred calls.
- Telemetry: `slog` (file + stdout, `internal/platform/telemetry/logging`) and OTel traces through `autoexport` (`internal/platform/telemetry/tracing`). `cmd.Setup` builds both.

### Config

- `internal/platform/config`: the value comes from the flag, else from the option default. There is no env adapter and no config file.
- Global flags are root persistent flags: `--db-path`, `--docling-url`, `--openai-url`, `--xberg-url`, `--log-path` (`GlobalOptions`, `RegisterFlags`, `ResolveGlobal`).
- "Flag is set" means `pflag.Flag.Changed`, not a value compare. Reason: pflag has no nil flags.
- Options with `IsPath` resolve one time to an absolute path against the process cwd (`ResolvePath`).
- Resolution occurs only in `RunE`, after cobra parses the flags. No `init()` registration. Reason: explicit order, no hidden control flow.
- Step flags stay local to their command (`--input-dir`, `--collection-name`, `--collection-weight`, `--model`, `--dim`, `--flush`).
- Embed provider: `--xberg-url` set → Kreuzberg, else `--openai-url` set → OpenAI-compatible, else error. Reason: the URL defaults are never empty, so only `Changed` prevents a nil client.
- The only env input is `RAG_CLI_API_TOKEN`. `serve mcp` requires it.

### Extraction

- Use docling-serve, not Kreuzberg. `internal/platform/extraction/docling` sends the file to `/v1/convert/file` and maps the `DoclingDocument` JSON.
  Reason: Kreuzberg wraps docling. A direct docling client removes one layer and gives more structure.
- Domain model `internal/extract/step/extract_model.go`: `Node` is a tagged struct (`Kind` + one non-nil payload pointer) with real `*Node` links.
  Reason: the model must go through `encoding/json`. Interface unions do not unmarshal.
- `Parent` has the tag `json:"-"` to prevent cycles. There is no second "consumer" node type. The unexported raw structs in `unmarshal.go` are the adapter boundary.
- Principle: the domain model keeps all data that the source supplies. The adapter lifts a poor source up to the model.
- `Provenance` is a slice for each node (page, `BBox` with top-left origin, `CharSpan`). `ContentLayer` is a field on each node.
- Label map in `normalize.go`:
  - `text`, `paragraph` → paragraph. `title`, `section_header` → heading (level 1–5).
  - `list_item`, `code`, `formula`, `caption`, `footnote` → own kinds with own content structs.
  - Group label `list` → list, `inline` → group. Tables and pictures → own kinds.
  - Other known labels → `KindUnsupported`.
- The extract side fails loud on unknown input. The chunk side is tolerant. There is no `KindUnknown`.
- `graph.go` `wireGraph`: docling has no `#/body` node, so its children become roots (many roots for each doc).
- Docling gives captions and footnotes the parent `#/body`. The adapter moves them under the host table or picture (host `captions`/`footnotes` refs) and removes them from the roots.
- Source walk (`internal/platform/extraction/source`): recursive, skips hidden directories, uses an extension allowlist (PDF, Office, ODT, md/txt, HTML, AsciiDoc, LaTeX). No CSV, media or email.
- `RunExtract` tolerates a failure of one doc: `ErrExtractionFailed` skips the doc, 10 consecutive failures stop the run. DB errors stop the run at once.
  Reason: unattended runs over large directory trees.
- Dedup on content `sha256` (`ExistsDoc`). A duplicate is skipped (`ErrDuplicateDoc`). A duplicate with a different collection name gets a warning and stays in its first collection.
- Two hand-mirrored type systems: `internal/extract/step` and `internal/chunk/step`. A new node kind touches `normalize.go`, `extracted_doc_sink.go` (`marshalContent`), `extraction_source.go` (`buildMap`), `chunk.go` (`walk`) and both content structs.
- Dev only: `--dump-dir` writes raw HTTP requests and responses (`httpclient.NewDump`). The docling client timeout is 1800 s.

### Chunk step

- `internal/platform/sqlite/chunk/extraction_source.go` builds the tree in two passes: `buildMap` (flat map), then `wireGraph` from `parent_id`. Rows have `ORDER BY en.id`.
  Reason: the FK `parent_id` is the single source of truth. No implicit coupling to the insert order.
- Docs that have chunks are skipped (`DocAlreadyChunked`). `chunk --flush` deletes `chunk_nodes` and `chunks` in one transaction, then the rows of all `embeddings_*` tables.
- `walk` (`internal/chunk/step/chunk.go`) is a DFS over all roots with one shared heading stack. Docling headings are flat siblings, so a level compare pops the stack.
- Breadcrumb = heading texts joined with `" > "`.
- Candidates for each kind:
  - paragraph → one content candidate.
  - group (docling `inline`) → one candidate. Fragments join with a space only where the join has no whitespace. Reason: docling cuts one paragraph at each bold or italic span.
  - list → one candidate, `marker + " " + text` for each item, joined with `\n`. For an empty item text, `inlineText` joins the fragments of the inline group child.
  - table → one candidate. Rows use `|` separators, one header row from column-header cells, no `---` row, no padding. A merged cell repeats in each cell it covers. Data rows start after the last header row. Caption and footnote children are appended.
    Reason: a header row is a strong prior for LLMs and embedding models. Padding uses tokens for no value.
  - code, formula → one standalone candidate, never split, raw text without fences.
  - caption, footnote, list_item that no host consumes → warning, then drop.
- Typed chunks: `chunks.type` is `content`, `generic` (group), `list`, `table`, `code` or `formula`. Each chunk has exactly one type. Reason: the render step needs it, and it costs nothing.
- `mergeCandidates`: only paragraph candidates merge.
  - Same breadcrumb and under the budget (1000, `tokenCount` = `len`) → merge with `\n\n`. A breadcrumb change or a budget overflow → flush.
  - An oversized single candidate becomes its own chunk. It is never dropped.
  - Container, code and formula candidates always become their own chunk.
- Content chunk text = `breadcrumb + "\n\n" + body`. Reason: more signal for FTS, the embedding model and the LLM.
- `mergeSmallChunks` (`merge_small_chunks.go`): a content chunk under 75 chars merges into the next content chunk. Direction is next only, with max one cascade. The result keeps the breadcrumb and `Position` of the next chunk.
- Settled, change only with eval numbers: containers do not flush the content buffer. `mergeSmallChunks` does not renumber `Position` (gaps are harmless, IDs stay stable). `candidatePos` stays doc-global.
- Accepted: a heading that contains `" > "` can look the same as a nested breadcrumb. The merge can then cross a section boundary.

### Retrieval text and render text

- `chunks.text` is the index text for FTS and embeddings. The chunk step computes it one time.
- Render text for a consumer comes at query time from the raw `extraction_nodes`, through `chunk_nodes`.
- `chunk_nodes(chunk_id, extraction_node_id, position)` references `extraction_nodes.id` (surrogate PK). `node_id` is unique only in one extraction. `InsertChunk` is `:one ... RETURNING id`.
- Reason: formatting at extraction (code fences, markdown) pollutes the FTS tokens, cannot be undone and looks the same as author text.
- Render rule: re-derive only kinds whose text is a pure function of the node data (paragraph, table, caption, footnote, code, formula, list item).
- Do not replay chunk-step policy (group or list stitch) at render time. Reason: `extractions` has no extractor identity column.
- Current state: `ChunkRenderer.RenderChunks` (`internal/platform/sqlite/retrieval/retrieval_renderer.go`) does nothing. Consumers get `chunks.text`.

### Embed step

- One OpenAI-compatible client (`internal/platform/embedclient/openai`, `POST /v1/embeddings`) for Ollama (dev) and vLLM (prod). Reason: one adapter for two backends.
- The Kreuzberg client (`--xberg-url`) stays as a legacy option. It takes model and dim from the response.
- Model `bge-m3`, dim 1024 (justfile). Reason: the documents are German and English. A cross-lingual benchmark gave near-perfect results for `bge-m3` and `qwen3-embedding:0.6b`.
- One vec0 table for each model and dim: `embeddings_<model>_<dim>`, with `:` and `-` replaced by `_` (`sqlite.vecTableName`).
  Reason: vec0 fixes the vector width at create time, and vectors of different models are not comparable.
- The write path `SetupVecTable` creates a missing table. The read path `LookupVecTable` never creates one and returns `VecTableNotExistErr`.
  Reason: a typo in `--model` must fail. It must not return zero hits from a new empty table.
- A chunk needs an embedding for model X when X's table has no row for it (`NOT EXISTS`, hand-built SQL in `internal/platform/sqlite/embedding/chunk_source.go`).
  Reason: a new model backfills all chunks with no special code. There is no `chunks.embedded` column.
- Table names come from `fmt.Sprintf` without escapes, by decision. Reason: `model` comes from a local flag, and that user already has file access to the DB.
- Batch size is 10. Chunks with `len(text) <= 1` are skipped.
- `embedWithFallback` (`internal/embedding/step/embedding.go`): on a model error, cut the batch in half and try again. A chunk that fails alone gets a warning and is skipped.
  Reason: Ollama/bge-m3 returns NaN for some batches. Graceful degradation is more robust than content filters.
- A transport failure wraps `step.ErrProviderUnreachable` and stops the run at once, without a bisect. A non-200 response stays on the model-error path.
- Clients compare the response count with the chunk count and the vector dim with the configured dim. The OpenAI client maps vectors by the response `Index`.

### Storage and schema

- SQLite through `mattn/go-sqlite3` (cgo, build tag `fts5`) and `sqlite-vec` (`asg017/sqlite-vec-go-bindings/cgo`).
- sqlc generates `internal/platform/sqlite/querries` from `queries.sql` and `sqlc_schema.sql`. The directory name has a typo.
- DSN: WAL, `_busy_timeout=5000`, `_synchronous=NORMAL`, `_foreign_keys=ON`. FKs are enforced. Max 8 open connections.
- Tables: `documents`, `collections`, `chunks`, `chunk_nodes`, `extractions` (with `mime_type`), `extraction_nodes` (`node_id`, `parent_id`, `kind`, `layer`, `content_json`, `provenance_json`).
- `manual_schema.sql`: `chunks_fts`, FTS5 external content on `chunks`, tokenizer `unicode61 remove_diacritics 2`. `fts_trigger.sql`: insert, update and delete triggers.
- All DDL is `CREATE ... IF NOT EXISTS` and runs on each `NewConn`. There are no migrations.
- The FTS `rebuild` runs only for `NewConn(..., true)`, which only the `chunk` command uses.

### Collections

- One collection for each document: `documents.collection_name NOT NULL` → `collections(name PK, weight REAL CHECK(weight >= 0))`.
  Reason: tags for many collections solved no real problem and made the boost harder (MAX over tags).
- `collections` stays a separate table. `documents` has no weight copy. Reason: one UPSERT changes the weight of all docs of the collection atomically.
- `extract` requires `--collection-name` and `--collection-weight`. `SaveExtractedDoc` does the UPSERT for each new doc.
- A collection is provenance and trust, doc-level, with a weight. Topic tags are a different concept and do not exist. A `/` in a name is only a naming convention.

### Retrieval and ranking

- `RunRetrieval` (`internal/retrieval/step/retrieve.go`): strategy `fts`, `ann` or `hybrid` → hydrate → render.
- Ports: `TopKRetriever`, `CollectionWeigher`, `ChunkHydrator`, `ChunkRenderer`, `EmbedClient`.
- FTS (`fts_retrieval.go`): terms longer than 2 runes, each quoted, joined with OR, plus the full query as a phrase. Bound parameter, `ORDER BY bm25`.
- ANN (`ann_retrieval.go`): vec0 `MATCH` on the table of the query model, `ORDER BY distance`.
- Hybrid (`runHybrid`):
  - FTS and ANN each fetch `k*4`.
  - `rrfMerge` (`rrfK = 60`) on plain IDs, with no truncation inside.
  - `collectionRerank` multiplies the score by the collection weight, sorts again and sets `Rank` again.
  - Then truncate to `k`. Equal scores sort by lower ID first.
- Multiplicative weight, not additive and not tiered.
  Reason: query-log data showed dual-hit RRF scores (about 0.026–0.033) at about two times the single-hit scores (max 0.0164).
  A weight of 0.6 keeps a strong match above a weak one, 0.5 demotes it, and no extra constant is necessary.
- The hybrid result drops a chunk that has no weight.
- `rrfMerge` stays DB-agnostic on IDs. A boost that needs chunk attributes runs after hydration on an over-fetched pool.
- Hydration (`retrieval_chunk_hydrator.go`): one `RetrievalChunksByIDs` query joins doc name, collection name and weight live. There is no startup cache.
- Output order follows the input IDs. Missing rows are skipped. Zero hits give an empty result, not an error.
- `RetrievedChunk`: `ID`, `Rank`, `DocTitle`, `Position`, `Score`, `Breadcrumb`, `Text`, `CollectionWeight`, `CollectionName`.
- `Score` is bm25 for FTS, distance for ANN and the weighted RRF score for hybrid.
- No cross-encoder or Python rerank stack. Use heuristics until eval numbers show that they are not sufficient.

### Serve and MCP

- `cmd/cli/cmd/serve`: `cmd.go` (flags), `wiring.go` (deps built one time, sealed `embedBackendConfig`), `tools.go`, `middleware.go`, `server.go`.
- One MCP tool `rag` (params `retrieval_query`, `k` from 1 to 10), always hybrid. Reason: one tool with internal routing is simpler for the client than one tool for each strategy.
- `mark3labs/mcp-go` streamable HTTP on `/mcp`, default bind `127.0.0.1:8808`, `otelhttp` wrapper. Timeouts: read and write 30 s, idle 120 s.
- `BearerAuth` compares the token in constant time and wraps the full handler.
- On SIGINT or SIGTERM, cleanup and `http.Server.Shutdown` use a new 10 s context.
- The handler recovers from panics and returns tool errors, not Go errors.
- Query log: JSON lines in the `--query-log` file (query, k, strategy, chunk IDs and scores). Span attributes: chunk IDs, char counts, scores.
- Section expansion stays internal first, with no `expand` param. Reason: an API param is a compatibility commitment.

### Eval

- Draft golden set `eval/golden.json` (not tracked): 33 queries, 3 of them `no_answer`.
- Format: `{id, query, expect:[{doc, breadcrumb_contains?, text_contains?}], note, no_answer?}`. One matching expectation = hit.
- Expectations reference content (doc name + breadcrumb or text substring), never chunk IDs. Reason: IDs change at each re-chunk.
- Rule: no more behaviour changes in chunk or rank code without eval numbers.

### Tests

- `just test` = `CGO_ENABLED=1 go test --tags "fts5" ./...`. `just lint` = `go vet` + `golangci-lint`. CI runs build, test and lint for master and PRs to master.
- Use the `just` recipes, not raw `go` commands.
- Pure logic: hand-written fakes in the step packages. HTTP: `net/http/httptest`.
- SQL logic: a real temp-file DB through `internal/platform/sqlite/sqlitetest`, not `:memory:`. Reason: each pooled connection to `:memory:` gets a separate empty DB.
- Expected float scores use runtime math (`rrfScore` helper), not constant expressions. Reason: constant folding rounds differently.

### Deferred, not built

- Graph engine (`docs/graph-design.md`): frontmatter `name`, `description`, `relations`; graph and embeddings are additive. Design only.
- Pipeline mode: one YAML file for extract → chunk → embed. Parked.
- Production telemetry beyond traces and the query log. Docling confidence scores are not stored.

## Consequences

Known limits and open bugs in the code (work items are in [the backlog](../requirements/backlog.md)):

- The render step does nothing. Consumers see the index text, with the breadcrumb prefix and the duplicate crumb lines from `mergeSmallChunks`.
- Non-content chunks (`generic`, `list`, `table`, `code`, `formula`) have no breadcrumb in `chunks.text`. FTS and embeddings do not see their section.
- `buildMap` maps pictures to `KindUnsupported`. For a root picture, its captions reach `walk` and drop with a warning.
- A picture or code node in an inline group drops with its captions. The log shows only a generic group warning.
- Docling `ordered_list` and other unsupported groups become `KindUnsupported`. Their `list_item` children reach `walk` as orphans and drop.
- The parser reads the bold and italic `formatting` (`rawFormatting`) and then discards it.
- No size guard before the embed API. An oversized code, formula or table chunk can truncate silently in the model.
- Some short German chunks get NaN from Ollama/bge-m3 also alone. They stay without an embedding (16 of 1656 in the last full run).
- Single-token `generic` chunks exist. bm25 length normalization can put them at the top of FTS results.
- An empty heading text becomes `MISSING_TITLE_PLACEHOLDER` in the breadcrumb.
- A changed file has a new `sha256`, so `extract` adds a second document with the same name. The old document and its chunks stay.
- `GetLatestExtractionOfDoc` has no "latest" filter and returns the nodes of all extractions of a doc. This is latent while each doc has one extraction.
- `documents.sha256` has no unique index.
- A re-run of `extract` with only duplicates does not change the collection weight. The UPSERT is in `SaveExtractedDoc`, which duplicates do not reach.
- The chunk step has no tolerance for a failure of one doc. One error stops the run.
- Collection weight applies only to `hybrid`. The CLI strategies `fts` and `ann` ignore it.
- `serve/cmd.go` calls `otelShutdownFunc(ctx)` with the cancelled signal context. Each graceful stop logs "context canceled".
- `httpclient` dump: file names have one-second resolution with `O_APPEND` (collisions). Dump errors are discarded. The response-dump warnings log `reqFileErr`.
- The `justfile` `inspect` recipe calls a command that does not exist.
- Cobra placeholder `Short`/`Long` text remains on root, `extract`, `embed`, `retrieve` and `serve`.
- Docling supplies the provenance page for only a small part of the nodes.
