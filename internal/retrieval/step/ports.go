package step

import "context"

type TopKRetriever interface {
	TopKByANN(ctx context.Context, query Query, k int64) (RetrievedChunkIDs, error)
	TopKByFTS(ctx context.Context, query string, k int64) (RetrievedChunkIDs, error)
}

type ChunkHydrator interface {
	HydrateChunks(ctx context.Context, chunkIDs RetrievedChunkIDs) ([]RetrievedChunk, error)
}

type EmbedClient interface {
	EmbedQuery(context.Context, string) (Query, error)
}
