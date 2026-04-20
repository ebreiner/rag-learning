package step

type EmbeddingToSave struct {
	ChunkID int64
	Vector  []float64
}

type ChunkToEmbed struct {
	ChunkID int64
	Text    string
}
