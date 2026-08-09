package step

type RetrievedChunk struct {
	Rank     int64
	DocTitle string
	Position int64
	Text     string
}

type RetrievedChunkIDs []int64

type RetrievalStrategy int

const (
	Hybrid RetrievalStrategy = iota
	FTS
	Embedding
)

type Query struct {
	Vector []float64
	Dim    int64
	Model  string
}
