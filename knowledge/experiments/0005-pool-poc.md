---
type: experiment
title: Pool PoC with saved queries and an MCP endpoint for each pool
status: open
date: 2026-10-23
tags: [pools, poc, web-ui, mcp, experiment]
---

# Pool PoC with saved queries and an MCP endpoint for each pool

Branch `claude_experimental`. Throwaway code: the branch runs as the production instance for some weeks, and it does not go to `master`. The target design for later is [knowledge-pools](../designs/knowledge-pools.md).

## Goals

Learn how domain experts select knowledge for a pool, and what a pool changes in the answers. Questions that we hope to answer (more questions will come):

- Can domain experts write useful pool queries, and which metadata do they miss?
- Is a table to browse the documents better than SQL, or does each need the other?
- Does a small pool give better answers than the full DB for the same question?
- Do users make many small pools or few big pools?
- With several `/mcp/<name>` endpoints in one client, does the model select the correct pool?

## Setup

General:
- All sources go into one rag-cli DB, as now.
- A small web UI in `serve`, with the existing static token.
- `/mcp` stays the full DB with all tools, unchanged. The pool endpoints are additional.

Pool definition:
- A pool has a `name`, a `description` and a `query`. All three are required.
- `name`: `^[a-z0-9-]{1,40}$`. `ui` and `mcp` are reserved.
- `query`: read-only SQL on the view `pool_documents` (`id`, `source_name`, `rel_path`, `title`, `url`, `metadata`). `title` is `doc.title` or the file name, `url` is `doc.url`. `NewConn` drops and creates the view at each start, so a change of the view reaches existing DBs. It returns one column of document IDs. The view has no flattened metadata keys: the keys are flat and contain dots, so users quote them: `json_extract(metadata, '$."doc.freshness"')`. Without the quotes, the result is always NULL. If that is too difficult, it is a finding.
- Document IDs do not survive an `ingest-clean`. A pool query selects by `source_name`, `rel_path` or metadata, not by ID values. The UI says this. A later "select rows in the table" feature generates `rel_path IN (…)`.
- `description`: the instruction text of the pool's MCP server, and the first part of the description of its `rag` tool. Many clients do not show server instructions to the model, and the model selects a pool by this text.

Storage and state:
- Pools are in the DB, in a table `pools` (`name`, `description`, `query`, `state`). They survive a restart and a crash of `serve`.
- The ingest does not delete the table. `just ingest-clean` deletes the DB file, and with it the pools.
- A pool is `running` or `stopped`. A stopped pool keeps its definition, and its endpoint gives `503` with a message. An unknown pool gives `404`. Delete is a separate operation.
- At start, `serve` starts each pool with state `running`. The pool queries run in the background after the start, so slow queries do not delay `serve`. Until its query is done, an endpoint gives `503` ("starting"). A pool query that fails does not stop `serve`: the UI shows the error, and the endpoint gives `503`.
- JSON export and import are for a new DB, not for a restart. One file holds all pools: `version`, and for each pool `name`, `description`, `query`. The file has no state. An import replaces pools with the same name, keeps the others and starts each imported pool as `running`. It saves all pools in one transaction and runs their queries in the background, because N queries of max. 10 s can take longer than the `WriteTimeout`. Unlike a save, an import also keeps a pool whose query fails: the pool list shows its error. A file with a name two times is refused.
- Pool changes (save, start, stop, delete, import) run one at a time, so the DB and the endpoints agree. A change waits while an import or the start resolves queries.
- In the UI API, start, stop and delete of an unknown pool give `404`.

Resolution and filter:
- The pool query runs at `serve` start, at save, at import and at the operation start. The result is a list of document IDs in memory. `serve` already needs a restart after an ingest, so the list cannot change while `serve` runs.
- Retrieval gets the list as a parameter (`d.id IN (SELECT value FROM json_each(?))`). The user SQL never goes into the retrieval SQL.
- The filter applies to `rag`, `read`, `links_out` and `links_in`, also to the totals of the closed link groups.
- Query plans with the filter, on a copy of the dev DB (734 documents, 7700 chunks), see [sqlite-query-plans](../concepts/sqlite-query-plans.md): FTS keeps `chunks_fts` as the outer loop, then primary-key lookups on `c` and `d`, then the `json_each` list. The ANN subquery reads `d` by rowid from the list, then the covering index `chunks_document_id_position`. The links queries use index lookups only.
- The query runs without the request context, so a closed browser does not leave a pool broken.

MCP endpoint of a pool:
- `/mcp/<name>`, same token. Each pool has its own `MCPServer` and streamable handler, because the tool descriptions differ for each pool. A stop, a start or a save of a running pool replaces its handler. mcp-go v0.46 uses session IDs without server state, so a client keeps its session ID on the new handler. Open GET streams on the old handler stay open until the `WriteTimeout`.
- Tools: `rag(retrieval_query, k)` and `read(chunk_id, mode, n)`. No `catalog`, and no `sources` or `metadata` arguments: the pool replaces them. A filter such as `doc.freshness = current` goes into the pool query.

User SQL safety:
- A separate read-only connection (`mode=ro`, `_query_only=1`) with its own driver name, not the shared pool of the MCP tools.
- The protection is an SQLite authorizer on that connection. SQLite asks it for each statement at prepare time. It permits only `SELECT`, `READ`, `FUNCTION`, `RECURSIVE` and `PRAGMA data_version` (FTS5 reads it internally), and refuses all other actions.
- The limits of the connection: `SQLITE_LIMIT_ATTACHED` 0, `SQLITE_LIMIT_LENGTH` 256 KiB for one value, `SQLITE_LIMIT_COLUMN` 64.
- The wrap `SELECT * FROM (<user SQL>)` is not a protection. `mattn/go-sqlite3` prepares all statements of a string and steps only the last one, so `SELECT 1); <statement> (` controls that last statement. Before the authorizer, this gave writes through `ATTACH 'file:<db>?mode=rw'`, new files and `PRAGMA hard_heap_limit` for the full process (security review). The tests `TestSandbox` in `internal/platform/sqlite/pool/pool_test.go` keep these attacks out. A read escape such as `SELECT 1) UNION SELECT name FROM sqlite_master …` still works, and the threat model permits reads.
- A time limit of 10 s through the context, below the `WriteTimeout` of 30 s. The UI shows max. 500 rows and max. 5 MiB of text.
- Checks at save: one integer column, values that exist in `documents`, a warning for an empty pool. A query that fails is not saved.
- Errors of the user SQL (syntax, authorizer, size, time limit) give `400`, other errors give `500`.

UI:
- One HTML page with `go:embed`, plain JS, no CDN, no build step, and a JSON API under `/ui/api/`.
- The page loads without the token and asks for it. It sends the token as the `Authorization` header. `/ui/api/*` and `/mcp*` need the token. The page keeps the token in `sessionStorage`, so it is gone when the tab closes.
- Corpus text (titles, metadata) goes into the page only with `textContent`, never with `innerHTML`.
- When a query result is one integer column, the page shows the source, the title and the URL of each ID, so the user sees what the pool selects. A URL from the corpus becomes a link only for `http` and `https`.

Logs:
- The pool name, the trace ID and the span ID go into the `query` and `tool_call` lines of `query.log`. The pool name also goes into the `rag` span (`query.pool`) and into the HTTP span of each request to `/mcp/<name>`.

Telemetry (local OpenWebUI + OpenRouter, outside the repo):
- `query` and `tool_call` lines have `trace_id` and `span_id` of the MCP request. A collector reads `query.log`, so the lines join the trace of the chat turn.
- OpenWebUI sends `traceparent` to rag-cli. An OpenWebUI filter sends `trace.trace_id`, `trace.parent_span_id` and `session_id` to OpenRouter, and OpenRouter Broadcast exports the generations into the same trace. One chat turn is thus one trace: model calls with tokens and cost, MCP calls, retrieval spans and query log lines.

Run 1 (`20261024T111748Z`): one question × 2 models (`deepseek/deepseek-v4.1-flash-1017`, `z-ai/glm-5.3-flash`) × 3 arms × 3 repeats = 18 chats, in random order:
- The question: an open product question ("a customer has a new dynamic electricity tariff, how can he use it in the product"). The answer is in one manual page; the release version of the feature is in a changelog.
- `pools-all`: 4 pool endpoints, wiki current (187 documents), wiki archive (338), product manual (199), changelogs (10). Disjoint, together all 734 documents. Tools `rag` and `read` on each endpoint.
- `pools-ec`: only the manual and the changelog endpoints (209 documents).
- `big`: `/mcp`, all 734 documents, tools `rag`, `read` and `catalog` with the filter arguments.
- `pools-all` and `big` differ in two things at the same time: the split and the tool set. A difference cannot be given to one of them alone.
- System prompt: the one of the manual UI runs before (user setting plus the OpenWebUI memory block). OpenWebUI adds the MCP server instructions (the pool descriptions).
- A runner script outside the repo sends each chat through the OpenWebUI API with its own `traceparent`. OpenWebUI 0.12 runs the tool loop only for a request with a chat ID and a message ID, so the runner creates the chat first and polls it for the answer. Without that, the API returns only the first model answer (`finish_reason: tool_calls`).
- Numbers from ClickStack for each trace ID. Answers in a review file for people to rate; a text search for the key facts.

Build order, each step usable alone:
1. Table to browse the documents.
2. SQL box with save, the `pools` table, export and import.
3. `/mcp/<name>` with the filter and the reduced tools, the pool name in the logs.
4. Pool list with the states and the operations view, start, stop and delete.

Evidence to collect:
- The pool name in each `query.log` line.
- Short talks with the users after one or two weeks.

Known limits:
- Everybody with the token sees all documents and all pools. Acceptable for a trusted test group.
- In OpenWebUI, only an admin adds a tool server. Each user gets a set of fixed pool names as "their" endpoints. The URL and the token stay stable, and the user changes the query behind the name.
- No eval for each pool. Add it only if an expert writes golden questions.
- `read` tells a client that a chunk ID outside its pool exists ("chunk not in the filter" against "chunk not found").

## Result

Run 1 (`20261024T111748Z`), mean of 3 chats, single values in brackets:

| Model / arm | Model calls | Input tokens | Cost USD | `rag` calls | `read` calls | Read chars |
|---|---|---|---|---|---|---|
| DeepSeek / big | 6.0 (7/4/7) | 81k | 0.0062 | 9/4/6 | 3/1/3 | 17k |
| DeepSeek / pools-all | 7.3 (9/6/7) | 137k | 0.0070 | 9/6/8 | 4/3/3 | 25k |
| DeepSeek / pools-ec | 5.3 (4/5/7) | 63k | 0.0078 | 4/5/7 | 2/2/3 | 14k |
| GLM / big | 3.7 (3/5/3) | 22k | 0.0010 | 2/2/2 | 1/2/1 | 12k |
| GLM / pools-all | 4.7 (3/7/4) | 46k | 0.0028 | 2/6/3 | 1/4/1 | 9k |
| GLM / pools-ec | 3.7 (4/4/3) | 27k | 0.0020 | 3/3/2 | 1/2/2 | 17k |

- Key facts of the manual page (source of the data, token, history limit, unit) in 18 of 18 answers.
- The release version of the feature (only in a changelog): 12 of 12 with pools, 1 of 6 with `big`.
- First model call (system prompt and tool descriptions): DeepSeek 1,338 tokens with 2 pools, 2,339 with 4 pools; GLM 1,190 and 2,165; `big` 1,058–1,182.
- Pool use with `pools-all`: one `rag` call into the current wiki for each model in 6 chats, none into the wiki archive. With `big`: 1 `catalog` call in 6 chats, no filter argument.
- Cached share of the input tokens: 36–77 % for each cell, and the cost per token changes with it.

## Findings

- The model has a bigger effect than the setup. GLM used 3–5 model calls and 0.001–0.003 USD for each chat, DeepSeek 4–9 calls and 0.006–0.008 USD, in all arms.
- Pool endpoints with a clear description make the models search each source that the question needs. With pools, all answers found the changelog fact; with one big endpoint, the models searched once, found the manual page and stopped. The question did not ask for the version, so this is completeness, not correctness.
- The routing by description works: the models did not search the wiki archive and searched the current wiki only once.
- Each pool endpoint costs about 500 input tokens in every model call (its own `rag` and `read` descriptions and schemas plus its instruction line). At 5 model calls that is about 2.5k tokens for each chat and pool. More pools can also lead to more model calls (DeepSeek: 7.3 with 4 pools, 5.3 with 2 pools), and one extra call costs the full context of 15–30k tokens.
- `catalog` and the filter arguments of `big` played almost no role.
- The cost for each chat is mostly (number of model calls) × (growing context): each call sends the full history again. Cache hits depend on the provider that OpenRouter selects, so cost in USD is noisy; calls and tokens are the better measure.
- The spread inside a cell is large (for example 4, 5 and 7 calls). 3 chats show a direction, not proof.
- API runs carry less context than UI runs: OpenWebUI adds its built-in tools only for UI requests (first call 1–2k tokens over the API, 7–8k in the UI). The cells are comparable with each other, not with UI chats.
- Full `read document` of a long changelog (about 21.6k characters) is the largest single waste seen in the manual runs.

## Recommendation

Open. Candidates for the next runs:
- More questions from the question set, not only one.
- One endpoint with one `rag` tool and a `pool` argument (enum with a one-line description for each pool): a pool would cost about 20–40 tokens instead of 500, and the descriptions stay visible for the routing.
- `big` without `catalog` and the filter arguments, to separate the split from the tool set.
