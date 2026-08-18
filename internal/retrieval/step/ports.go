package step

import "context"

type TopKRetriever interface {
	TopKByANN(query Query, k int64, ctx context.Context) (RetrievedChunkIDs, error)
	TopKByFTS(query string, k int64, ctx context.Context) (RetrievedChunkIDs, error)
}

type ChunkHydrator interface {
	HydrateChunks(chunkIDs RetrievedChunkIDs, ctx context.Context) ([]RetrievedChunk, error)
}

type EmbedClient interface {
	EmbedQuery(string, context.Context) (Query, error)
}
