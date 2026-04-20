package step

type TopKRetriever interface {
	TopKByANN(embedding []float64, k int64) (RetrievedChunkIDs, error)
	TopKByFTS(query string, k int64) (RetrievedChunkIDs, error)
}

type ChunkHydrator interface {
	HydrateChunks(chunkIDs RetrievedChunkIDs) ([]RetrievedChunk, error)
}

type EmbedClient interface {
	Embed([]string) ([][]float64, error)
}
