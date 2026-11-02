---
type: experiment
title: OpenWebUI Knowledge retrieval against rag-cli (level 1)
status: open
date: 2026-10-25
tags: [openwebui, retrieval, eval, hybrid, reranker, embedding]
---

# OpenWebUI Knowledge retrieval against rag-cli (level 1)

Question: how good is the built-in knowledge retrieval of OpenWebUI against rag-cli, with the same documents, the same extraction service and the same embedding model? Level 1 compares only the retrieval, with the rag-cli golden set and the document metrics of `rag-cli eval`. Level 2 (chat turns end to end) is not done. Related: [0005-pool-poc](0005-pool-poc.md), [eval-harness](../designs/eval-harness.md).

## Setup

- OpenWebUI 0.12.3, local container, Chroma vector DB.
- Documents: the 734 documents of the rag-cli DB, uploaded by a script (outside the repo) into one knowledge base for each source. Sidecars are not uploaded.
- Settings: extraction docling (the same docling service as rag-cli, but only for PDFs: OpenWebUI reads `.md` files with its own loader), embedding Ollama `bge-m3` (the same Ollama as rag-cli), character splitter with the Markdown header splitter, chunk size 1000 characters, overlap 100, hybrid search on, BM25 weight 0.5, Top-K 10, Top-K-Reranker 10, relevance threshold 0.
- Eval script (outside the repo): each golden query against `POST /api/v1/retrieval/query/collection` over all knowledge bases with `k = k_reranker = 10`. A chunk is mapped back to the rag-cli document name. Doc rank, doc r@n and doc MRR as in `internal/eval/step/eval.go` and `metrics.go`; `no_answer` queries are not counted (33 queries). Strategies `owui-hybrid` and `owui-vector` (hybrid off).
- Baseline: rag-cli run `20261022T192853Z` (hybrid doc r@1 0.697, doc MRR 0.739).
- The script also computes all metrics on the common set: the queries whose expected documents are all indexed in OpenWebUI, for both systems.

## Result

Run `20261025T065815Z` (708 of 734 documents indexed, no reranker):

| Strategy | doc r@1 | doc r@3 | doc r@5 | doc r@10 | doc MRR |
|---|---|---|---|---|---|
| owui-hybrid | 0.576 | 0.788 | 0.818 | 0.879 | 0.683 |
| owui-vector | 0.576 | 0.788 | 0.818 | 0.879 | 0.683 |
| rag-cli fts | 0.424 | 0.636 | 0.697 | 0.788 | 0.545 |
| rag-cli ann | 0.515 | 0.758 | 0.879 | 0.939 | 0.658 |
| rag-cli hybrid | 0.697 | 0.727 | 0.879 | 0.879 | 0.739 |

- Not counted as final: 26 documents were missing (see Findings), two golden queries (g27, g28) expect one of them.
- Per query: rag-cli hybrid misses g13, g14, g30, which OpenWebUI finds at ranks 2, 6, 4. OpenWebUI misses g15 (rag-cli rank 4) and g27, g28 (documents missing). Both miss g24.
- A second run `20261025T070817Z` on the same 708 files gave other numbers (doc r@1 0.545, MRR 0.656) and other scores: an embedding setting changed between the runs. The knowledge bases were then reset and uploaded again.
- After the new upload: 717 of 734 documents indexed (one source file is empty, 16 fail on the embedding, one PDF failed without a clear log entry).
- The run with the reranker `BAAI/bge-reranker-v2-m3` is open; its result goes here.

## Findings

- OpenWebUI hybrid search without a reranking model is plain vector search. BM25 and vector top k form an RRF union, then the "rerank" step scores the union by cosine similarity and keeps the top k; that is always the vector top k. `owui-hybrid` and `owui-vector` gave the same hit lists and scores for all 36 queries. BM25 only has an effect with a reranking model.
- Top-K-Reranker cuts the list also without a reranking model (default 3): the model then gets 3 chunks, in vector order.
- Ollama `bge-m3` returns NaN for some short inputs, at `/api/embed` and at `/v1/embeddings`, alone and in batches. Example class: a chunk of two short lines that starts with a level-4 Markdown heading; without the heading marks, and each line alone, the embedding works. OpenWebUI then refuses the whole document; the same 16 documents fail at each upload. rag-cli did not hit the bug, because its chunks start with the breadcrumb. In principle rag-cli can hit it with other texts.
- OpenWebUI sends the docling key as `X-Api-Key`; the header name cannot be configured. A proxy in front of docling must accept that header.
- OpenWebUI refuses an empty source file; rag-cli keeps it as a document without chunks.
- The reranker on CPU in the container takes about 20–30 s for each query (about 14 cores, 6 GB RAM). In a chat, each knowledge search would cost that much. rag-cli retrieval takes about 0.1 s plus the embedding.
- OpenWebUI ignores the sidecars: no `doc.*` metadata, no freshness, no links. A pool by metadata is only possible as a separate knowledge base.
- Setup mistakes that cost time: a model name without its Hugging Face organization (`bge-reranker-v2-m3`, also `BAAI//…`) does not load; the embedding engine field fell back to SentenceTransformers with the Ollama model name; `host.docker.internal` does not reach the PC from rootless Docker; an upload manifest from before a reset points to deleted knowledge bases.

## Recommendation

Open. Compare on the common query set when the reranker run is done.
