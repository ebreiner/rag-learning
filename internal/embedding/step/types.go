package step

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
	ChunkID int64
	Text    string
}
