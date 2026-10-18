---
type: design
title: Scopes for claude-unstable
status: draft
date: 2026-10-17
tags: [unstable, scopes, ports, architecture, backlog]
---

# Scopes for claude-unstable

This file groups the [backlog](../requirements/backlog.md) into scopes. Each scope is one subsystem or one coherent change.
For each scope it shows the ports and the domain types, the reasons, and the effect on the frozen eval. It shows no implementation.

## Naming

One term for the origin of a document, in all layers: **source**. Table `sources`, column `documents.source`, config key `sources:`,
domain type `source.Source`, MCP filter and result field `source`, span attribute `rag.source`.
The handover uses "data source" and "collection" for the same thing. Both map to "source".
The directory walk package `internal/platform/extraction/source` gets the new name `internal/platform/extraction/walk` (S2).

## Overview

| Scope | Subsystem | Backlog / RC | New packages | Schema change | Cycle |
|---|---|---|---|---|---|
| S0 | Chunk quality | `generic` chunks, carry-overs | none | no | 1 |
| S1 | Ranking pipeline | hybrid vs. ANN finding, RC7, RC15, old T14 | none in S0 form, stages in `retrieval/step` | no | 1 (fusion), 4 (reranker, freshness) |
| S2 | Source model | RC10, RC14, RC17, RC18, [0001](../decisions/0001-config-yaml-data-sources.md) | `internal/source` | yes | 2 |
| S3 | Ingest and document lifecycle | RC11, [0002](../decisions/0002-document-identity-derived-db.md), [0006](../decisions/0006-ingest-command.md) | `internal/ingest/step` | yes | 2 |
| S4 | Retrieval API and MCP surface | RC2–RC6, [0004](../decisions/0004-mcp-tool-surface.md), old T15, T17 | `internal/read/step`, `internal/catalog/step` | small | 3 |
| S5 | Access and telemetry | RC8, RC19, [0005](../decisions/0005-oauth-resource-server.md) | `internal/platform/auth` | no | 5 |
| S6 | Later | RC11 schedule, RC13, RC16 | `internal/rewrite/step` | no | later |

Dependencies: S4 needs S2 (metadata in results). S1 freshness needs S2 (dates). S3 needs S2 (sources). S0 and the fusion part of S1 need nothing, so they go first.

Target data flow after S2–S4:

```
config.yaml ─► source model (S2) ─► ingest plan (S3) ─► extract ─► chunk (S0) ─► embed
                                                                                   │
MCP search ─► candidates (FTS+ANN, filters) ─► hydrate ─► rank stages (S1) ─► render ─► result
MCP read   ─► neighbours / section / document ─────────────────────────────► render ─► result
MCP catalog ─► config (static) + DB counts (dynamic)
                      all tool calls ─► auth (S5) ─► spans with identity and filters (S5)
```

---

## S0 Chunk quality

**Goal:** give the index text of each chunk its section signal, and stop the split of normal paragraphs into small `generic` chunks.

**Ports:** no change. The work is inside `internal/chunk/step` (`walk`, `mergeCandidates`).

**Changes, one experiment each:**
1. Breadcrumb prefix for `generic` chunks, like `content` chunks.
2. `generic` candidates merge with neighbouring paragraph candidates under the same breadcrumb.
3. Optional: breadcrumb prefix for `list`, `table`, `code`, `formula`.

**Why first:** experiment [0001](../experiments/0001-eval-baseline.md) shows 66 % `generic` chunks, and all 5 full misses are `generic`. No schema change, so the eval stays untouched.

**Eval effect:** none on the eval code. Each run needs a full re-import.

---

## S1 Ranking pipeline

**Goal:** one ordered chain of rank stages after hydration. Each stage can be switched on and off, so the eval measures each stage alone.

**Today:** `runHybrid` does RRF and the source weight on IDs, before hydration. A stage that needs chunk text or metadata cannot run there.

**Ports and types** (`internal/retrieval/step`):

```go
// Candidate is a hydrated chunk with a score that each stage can change.
type Candidate struct {
	Chunk RetrievedChunk
	Score float64
}

// RankStage changes the order and the scores of the candidates. It must not add candidates.
type RankStage interface {
	Name() string
	Apply(ctx context.Context, query string, in []Candidate) ([]Candidate, error)
}

// Reranker is the port for a remote cross-encoder (RC7). The adapter calls the API.
type Reranker interface {
	Rerank(ctx context.Context, query string, texts []string) ([]float64, error)
}

type RetrievalDeps struct {
	// ... existing ports
	Stages   []RankStage // order = config order
	PoolSize int64       // candidates before the stages, > k
}
```

Stages, each a small type in the step package:
- `SourceWeightStage`: the current multiplicative weight, moved after hydration.
- `RerankStage`: wraps the `Reranker` port.
- `FreshnessStage` (RC15, needs S2): policy for each source, validity status before age, unknown date = neutral.
- `BreadcrumbBoostStage` (old T14): only if the eval shows a need after the reranker.

Fusion stays before hydration, because it works on two ID lists. Make it a parameter: `FusionConfig{RRFK int64, WeightFTS, WeightANN float64}`.

**Why:**
- The baseline shows hybrid r@1 = 0.27 against ANN r@1 = 0.40. A fusion weight is the cheapest first test.
- A chain of stages gives one place for RC7, RC15 and old T14. Each stage is measurable alone.
- The `Reranker` port takes texts, not chunks. Thus the adapter has no domain knowledge, and a fake is trivial.

**Eval effect:** none. `RunRetrieval` keeps its signature. Stages and fusion come from the command flags.

**Open question:** which rerank service runs locally? Ollama has no rerank endpoint. A TEI or vLLM container with a multilingual reranker is necessary for cycle 4.

---

## S2 Source model

**Goal:** one place that turns `config.yaml` and the sidecars into validated document metadata. All later scopes read this model.

**New package** `internal/source` (domain, no I/O):

```go
type Source struct {
	Name        string
	Description string
	Dir         string
	Attributes  map[string]any // defaults for each document
	URLTemplate string         // optional, e.g. "https://host/{{.RelPath}}"
	Freshness   FreshnessPolicy
	Weight      *float64 // optional (0003)
}

// DocMetadata holds the known meanings (RC17) as typed fields and the rest as Extra.
type DocMetadata struct {
	Title          string
	SourceURL      string // empty = no URL (RC5 marks it)
	Created        *time.Time
	Updated        *time.Time
	Language       string
	Validity       string // explicit validity status (RC15)
	Tags           []string
	Classification Classification
	Extra          map[string]any
}

type Classification struct { // RC18: required, no default
	CustomerData    bool
	PersonalData    bool
	MissionCritical bool
}

// Resolve merges source defaults and the sidecar, protects the system attributes,
// validates the known meanings and the required set. A failure rejects the document.
func Resolve(src Source, relPath string, sidecar map[string]any) (DocMetadata, error)
```

**Ports** (adapters in `internal/platform/...`):

```go
type ConfigLoader interface { // adapter: YAML file
	Load(path string) ([]Source, error)
}

type SidecarReader interface { // adapter: <file>.meta.json, nil map when no file
	Read(ctx context.Context, absPath string) (map[string]any, error)
}
```

**Why:**
- `Resolve` is a pure function: the full exporter contract (defaults, override, protection, validation, rejection) is in one place and has table tests.
- Typed fields for the known meanings: retrieval and ranking read `Updated` or `Validity` without map lookups and type checks.
- `Extra` keeps all other keys for RC5 ("all document metadata").

**Eval effect:** none in this scope.

---

## S3 Ingest and document lifecycle

**Goal:** one `ingest` command that brings the DB to the state of the directory: add, replace, delete, skip.

**Schema:** `documents` gets `source`, `rel_path`, `title`, `source_url`, `created`, `updated` and keeps `metadata_json`. `UNIQUE(source, rel_path)`. `collections` becomes `sources` (name, description, weight, last_ingested). `documents.name` stays the file name, so the eval adapter does not change.

**New package** `internal/ingest/step`:

```go
type ScannedDoc struct {
	Source     string
	RelPath    string
	AbsPath    string
	SHA256     string
	Metadata   source.DocMetadata
}

type ActionKind string // add, replace, delete, skip

type Action struct {
	Kind     ActionKind
	Doc      ScannedDoc // empty for delete
	OldDocID int64      // set for replace and delete
}

// Plan compares the directory with the DB. Pure function.
func Plan(scanned []ScannedDoc, indexed map[string]IndexedDoc) []Action

type Scanner interface { // adapter: directory walk + sidecar + source.Resolve
	Scan(ctx context.Context, src source.Source) ([]ScannedDoc, []Rejected, error)
}

type DocIndex interface { // adapter: SQLite
	Indexed(ctx context.Context, source string) (map[string]IndexedDoc, error) // key: rel_path
	Delete(ctx context.Context, docID int64) error // chunks, chunk_nodes, embeddings_*, extractions, FTS
}

type IngestDeps struct {
	Scanner   Scanner
	Index     DocIndex
	Extract   func(ctx context.Context, doc ScannedDoc) error // wraps RunExtract for one doc
	Chunk     func(ctx context.Context) error                 // existing RunChunk
	Embed     func(ctx context.Context) error                 // existing RunEmbed
}

func RunIngest(ctx context.Context, sources []source.Source, deps IngestDeps) (Report, error)
```

**Replace order:** extract the new version first (slow, docling), then delete the old document and insert the new one in one transaction. A failed extraction keeps the old version.

**Why:**
- `Plan` is pure: all lifecycle rules have table tests without a DB.
- Chunk and embed need no change: they already pull "not chunked" and "not embedded" work. Thus `ingest` is incremental at no cost.
- A rejected document (RC18) is in the `Report`. It does not stop the run.

**Eval effect:** none, if `documents.name` keeps the file name. This is a schema change, so compact after this scope.

---

## S4 Retrieval API and MCP surface

**Goal:** the three tools of [0004](../decisions/0004-mcp-tool-surface.md) with filters, rich results, a real renderer and a read tool.

**Retrieval request** (`internal/retrieval/step`):

```go
type SearchRequest struct {
	Query       string
	K           int64
	Sources     []string          // empty = all (RC4)
	Metadata    map[string]string // equality filters on DocMetadata keys
}

type Filter struct { // the form that the adapters get
	Sources     []string
	Metadata    map[string]string
}

type TopKRetriever interface {
	TopKByANN(ctx context.Context, q Query, k int64, f Filter) ([]ScoredChunkID, error)
	TopKByFTS(ctx context.Context, q string, k int64, f Filter) ([]ScoredChunkID, error)
}

func RunRetrieval(ctx context.Context, req SearchRequest, strategy RetrievalStrategy, deps RetrievalDeps) ([]RetrievedChunk, error)
```

`RetrievedChunk` gets `Doc DocRef` (source, title, rel_path, source URL, created, updated, metadata).
The ANN adapter filters with an over-fetch and a post-filter. A vec0 partition key on the source is an option later.

**Renderer** (old T15): `ChunkRenderer` reads the nodes through `chunk_nodes` in one batch and renders each node kind. The index text stays for FTS and embeddings only.

**New package** `internal/read/step` (RC6):

```go
type ReadRequest struct {
	ChunkID    int64
	Mode       ReadMode // neighbours, section, document
	Window     int      // neighbours: chunks before and after
	MaxChars   int      // limit for all modes
}

type ChunkStore interface { // adapter: SQLite
	Neighbours(ctx context.Context, chunkID int64, before, after int) ([]StoredChunk, error)
	Section(ctx context.Context, chunkID int64) ([]StoredChunk, error) // same doc, breadcrumb prefix
	Document(ctx context.Context, chunkID int64) ([]StoredChunk, error)
}

func RunRead(ctx context.Context, req ReadRequest, store ChunkStore, render ChunkRenderer) (ReadResult, error)
```

**New package** `internal/catalog/step` (RC2, RC3):

```go
type SourceStats struct { // dynamic, from the DB
	Name          string
	DocCount      int64
	LastIngested  *time.Time
	WithoutURL    int64
	MetadataKeys  []string
}

type StatsSource interface {
	Stats(ctx context.Context) ([]SourceStats, error)
}

// Describe builds the tool descriptions and server instructions from static config only (0004).
func Describe(sources []source.Source) ToolTexts
```

**MCP layer** (`cmd/cli/cmd/serve`): three thin handlers. Each handler parses the arguments, calls one `Run*` function and maps the result to JSON. No logic in the handlers.

**Why:**
- `SearchRequest` replaces the loose parameters: filters can grow without new function signatures.
- Read is a separate step, not a retrieval option: it does not rank, it navigates from a known chunk.
- `Describe` is pure and static, so the descriptions never become stale (0004).

**Eval effect:** `RunRetrieval` gets a new signature. The searcher adapter in `cmd/cli/cmd/eval.go` must change (`eval-adapter:` commit). The eval core stays the same.

---

## S5 Access and telemetry

**Goal:** OAuth resource server (RC19a), identity in spans (RC19b), static token for development (RC19d), complete tool spans (RC8).

**New package** `internal/platform/auth`:

```go
type Identity struct {
	Subject string
	Name    string
	Issuer  string
}

type TokenVerifier interface {
	Verify(ctx context.Context, rawToken string) (Identity, error)
}
// adapters: StaticTokenVerifier, OIDCVerifier (JWKS from the configured issuer)
```

The middleware calls the `TokenVerifier`, puts the `Identity` into the context, and answers 401 with `WWW-Authenticate: Bearer resource_metadata="..."`. A separate handler serves `/.well-known/oauth-protected-resource` (RFC 9728).

RC8: one span for each tool call with query, filters, sources, result IDs, scores, delay and identity. The trace context of the client already continues (`otelhttp` + `TraceContext`).

**Why:** one port, two adapters: the static token is only a different `TokenVerifier`, not a second code path.

**Eval effect:** none.

**Blocker:** the PO supplies the test Keycloak realm.

---

## S6 Later

- RC13 query rewrite: port `QueryRewriter{ Rewrite(ctx, q string) (string, error) }` as the first stage before retrieval. Only if RC1 shows a gain.
- RC11 schedule and exporter trigger: a loop or an HTTP trigger around `RunIngest`.
- RC16: measure the delay of each stage in the spans, then optimize.
- Carry-overs that touch S0 (picture captions, size guard, NaN chunks, garbage filter) go into S0 experiments when the eval shows a need.

## Cycle plan

| Cycle | Experiments | Compaction trigger |
|---|---|---|
| 1 | S0.1 `generic` breadcrumb, S0.2 `generic` merge, S1 fusion weights | 3 experiments |
| 2 | S2 source model + S3 ingest | schema change |
| 3 | S4 tools, renderer, read | interface change of `RunRetrieval` |
| 4 | S1 stages: source weight as stage, reranker, freshness | reranker service |
| 5 | S5 auth and spans | PO realm |
