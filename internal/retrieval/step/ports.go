package step

type TopKRetriever interface {
	TopKByANN(query Query, k int64) (RetrievedChunkIDs, error)
	TopKByFTS(query string, k int64) (RetrievedChunkIDs, error)
}

type ChunkHydrator interface {
	HydrateChunks(chunkIDs RetrievedChunkIDs) ([]RetrievedChunk, error)
}

type EmbedClient interface {
	EmbedQuery(string) (Query, error)
}
