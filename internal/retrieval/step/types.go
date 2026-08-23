package step

type RetrievedChunk struct {
	ID       int64
	Rank     int64
	DocTitle string
	Position int64
	Text     string
}

type RetrievedChunkIDs []int64

type RetrievalStrategy string

const (
	Hybrid    RetrievalStrategy = "hybrid"
	FTS       RetrievalStrategy = "fts"
	Embedding RetrievalStrategy = "ann"
)

type Query struct {
	Vector []float64
	Dim    int64
	Model  string
}
