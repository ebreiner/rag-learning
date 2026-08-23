package step

import "context"

type EmbedClient interface {
	EmbedChunks(context.Context, []ChunkToEmbed) (EmbeddingsToSave, error)
}

type ChunkSource interface {
	NextChunks(ctx context.Context, limit int64) ([]ChunkToEmbed, error)
}

type EmbeddingsSink interface {
	SaveEmbeddings(ctx context.Context, toSave EmbeddingsToSave) error
}
