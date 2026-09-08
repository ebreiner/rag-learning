package step

import "errors"

var ErrProviderUnreachable = errors.New("embedding provider unreachable")

type EmbeddingsToSave struct {
	Embeddings []Embedding
	Model      string
	Dim        int64
}

type Embedding struct {
	ChunkID int64
	Vector  []float64
}

type ChunkToEmbed struct {
	DocID   int64
	ChunkID int64
	Text    string
}
