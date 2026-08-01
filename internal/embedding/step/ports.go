package step

type EmbedClient interface {
	Embed([]string) ([][]float64, error)
}

type ChunkSource interface {
	NextChunks(limit int64) ([]ChunkToEmbed, error)
}

type EmbeddingsSink interface {
	SaveEmbeddings(embeddings []EmbeddingToSave) error
}
