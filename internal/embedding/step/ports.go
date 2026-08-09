package step

type EmbedClient interface {
	EmbedChunks([]ChunkToEmbed) (EmbeddingsToSave, error)
}

type ChunkSource interface {
	NextChunks(limit int64) ([]ChunkToEmbed, error)
}

type EmbeddingsSink interface {
	SaveEmbeddings(toSave EmbeddingsToSave) error
}
