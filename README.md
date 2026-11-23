# rag-cli

rag-cli is a local RAG server for AI agents. It reads document folders, makes them searchable, and gives the search to
an agent through MCP.

## Idea

- Each knowledge source (a manual, a wiki, a file share) is one folder. An **exporter** writes this folder.
- rag-cli only reads the folders. It does not connect to the source systems.
- The DB is derived data. To change the content, export again and run a full ingest. There are no migrations.
- The agent gets three MCP tools: `rag` (search), `read` (open more text around a hit) and `catalog` (the sources and
  the metadata values for filters).
- A golden set and the `eval` command measure each change to the rank.

The product goal and the requirements are in the [handover](knowledge/requirements/rag-cli-handover.md). Start with the
[knowledge index](knowledge/index.md).

## Architecture

```
exporter ──► source folder ──ingest──► docling ──► nodes ──► chunks ──► embeddings + FTS5 ──► SQLite DB
            (documents +                                                                        │
             <file>.meta.json)                                                serve mcp ◄───────┘
                                                                     (rag, read, catalog)
```

- **Ingest:** docling-serve converts each document into a tree of nodes. The chunker makes chunks with a heading
  breadcrumb. An OpenAI-compatible endpoint or Kreuzberg makes the embeddings.
- **Storage:** one SQLite file with FTS5 and `sqlite-vec`. The document identity is the source name plus the path
  relative to the source folder ([0002](knowledge/decisions/0002-document-identity-derived-db.md)).
- **Retrieval:** two phases ([0013](knowledge/decisions/0013-retrieval-two-phases.md)). First, FTS and ANN find a pool
  and RRF merges it. Then the source weight applies, an optional remote reranker scores the pool, and the result is cut
  to `k`.
- **Filters:** by source (OR) and by metadata key (equality, AND)
  ([retrieval-filters](knowledge/designs/retrieval-filters.md)).
- **Links:** the sidecar can give the outgoing links of a document. The `rag` result shows the outgoing and incoming
  links ([document-links](knowledge/designs/document-links.md)).
- **Code:** ports and adapters. The domain steps are in `internal/<stage>/step`, the adapters in `internal/platform/`.
  The composition root is the `RunE` of each command in `cmd/cli/`. The details are in
  [0000](knowledge/decisions/0000-migrated-baseline.md) and [0011](knowledge/decisions/0011-config-file-and-composition-root.md).

## Requirements

- Go 1.26 with CGO (the build uses the tag `fts5`)
- [just](https://github.com/casey/just)
- a [docling-serve](https://github.com/docling-project/docling-serve) instance
- an embedding endpoint, for example Ollama with `bge-m3`
- optional: an OTLP endpoint for traces

## Configuration

1. Copy `config.yaml.example` to `config.yaml`. `config.yaml` is in `.gitignore`.
2. Set `docling.url` and the `embedding` block.
3. Add one entry in `sources` for each folder: `name`, `path`, `weight` and `attributes`.
4. Set `classification.personal_data` for each document, in the source `attributes` or in a sidecar.
5. For `serve`, set the bearer token in the env `RAG_CLI_API_TOKEN`, for example in `.env`.

The comments in [`config.yaml.example`](config.yaml.example) explain each key. Relative paths resolve against the
directory of the config file. Unknown keys are an error. Do not commit `config.yaml` or `.env`: they can contain
secrets.

## Commands

| Recipe | What it does |
|---|---|
| `just build` | build `./main` |
| `just ingest` | extract, chunk and embed all sources into the DB |
| `just ingest-clean` | delete the DB, then ingest all sources again |
| `just ingest-rechunk` | keep the extractions, chunk and embed again |
| `just retrieve '<query>'` | run one hybrid query and print the chunks as JSON |
| `just serve-mcp` | serve the MCP tools on `server.listen` |
| `just eval [<run>]` | run the golden set and compare with a baseline run |
| `just test`, `just lint` | run the tests and the linters |

Set `RAG_CONFIG` to use a config other than `config.yaml`. Also see `./main --help` for `read` and `catalog`.

## Write an exporter

An exporter is a program that writes the documents of one source into one folder. rag-cli does not care how the
exporter gets its data. The full rules are in the [exporter contract](knowledge/requirements/exporter-contract.md).

The short version:

1. Write each document as a file that docling can read, for example Markdown, HTML or PDF. Convert other formats, for
   example wikitext, to Markdown.
2. Give Markdown one top heading `# <title>`. Put all other headings below it.
3. Keep the relative paths stable between exports. The path is part of the document identity.
4. Optional: write a sidecar `<file>.meta.json` next to the document. It is one flat JSON object.
5. Export the full source into an empty folder, then run `just ingest-clean`.

Example: `process-list.md` and `process-list.md.meta.json`:

```json
{
  "classification.personal_data": false,
  "doc.title": "Process list",
  "doc.url": "https://wiki.example.org/Process_list",
  "doc.last_updated": "2026-09-30",
  "doc.freshness": "current",
  "links": [
    {"to": "processes/release.md", "text": "Release process", "section": "Delivery"}
  ]
}
```

- The sidecar keys go over the source `attributes`. The sidecar wins for each key.
- rag-cli requires only `classification.personal_data`. It shows all other keys in the result and in `catalog`, and the
  `metadata` filter can use them.
- Use the shared `doc.*` keys from the contract with the same meaning in all exporters.
- `links` is optional. A sidecar with `links` must have `doc.title`.

The design of the sidecar is in [sidecar-metadata](knowledge/designs/sidecar-metadata.md). The design of the links is
in [document-links](knowledge/designs/document-links.md).

## License

[MIT](LICENSE)
