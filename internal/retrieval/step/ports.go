package step

import "context"

type TopKRetriever interface {
	TopKByANN(ctx context.Context, query Query, k int64) ([]ScoredChunkID, error)
	TopKByFTS(ctx context.Context, query string, k int64) ([]ScoredChunkID, error)
}

type CollectionWeigher interface {
	WeighChunks(ctx context.Context, chunkIDs []ScoredChunkID) (map[int64]float64, error)
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
