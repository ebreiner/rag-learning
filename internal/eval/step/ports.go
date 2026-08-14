package step

type ChunkHydrator interface {
	HydrateChunks(chunkIDs []int64) ([]RetrievedChunk, error)
}

type TopKRetriever interface {
	TopKByANN(query Query, k int64) ([]int64, error)
	TopKByFTS(query string, k int64) ([]int64, error)
}

type EmbedClient interface {
	EmbedQuery(string) (Query, error)
}
