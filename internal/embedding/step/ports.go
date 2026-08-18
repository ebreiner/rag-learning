package step

import "context"

type EmbedClient interface {
	EmbedChunks([]ChunkToEmbed, context.Context) (EmbeddingsToSave, error)
}

type ChunkSource interface {
	NextChunks(limit int64, ctx context.Context) ([]ChunkToEmbed, error)
}

type EmbeddingsSink interface {
	SaveEmbeddings(toSave EmbeddingsToSave, ctx context.Context) error
}
