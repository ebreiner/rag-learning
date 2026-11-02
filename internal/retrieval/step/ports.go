package step

import "context"

type TopKRetriever interface {
	TopKByANN(ctx context.Context, query Query, k int64, filter Filter) ([]ScoredChunkID, error)
	TopKByFTS(ctx context.Context, query string, k int64, filter Filter) ([]ScoredChunkID, error)
}

type ChunkHydrator interface {
	HydrateChunks(ctx context.Context, chunkIDs []ScoredChunkID) ([]RetrievedChunk, error)
}

type ChunkRenderer interface {
	RenderChunks(ctx context.Context, chunks []RetrievedChunk) ([]RetrievedChunk, error)
}

type EmbedClient interface {
	EmbedQuery(context.Context, string) (Query, error)
}

// Reranker gives one score for each text, in the input order.
type Reranker interface {
	Rerank(ctx context.Context, query string, texts []string) (RerankResult, error)
}
