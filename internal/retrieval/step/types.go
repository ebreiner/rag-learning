package step

import "log/slog"

type RetrievalDeps struct {
	Logger           *slog.Logger
	Hydrator         ChunkHydrator
	Retriever        TopKRetriever
	EmbeddingsClient EmbedClient
	CollWeigher      CollectionWeigher
	Renderer         ChunkRenderer
}

type RetrievedChunk struct {
	ID               int64
	Rank             int64
	DocTitle         string
	Position         int64
	Score            float64
	Breadcrumb       string
	Text             string
	CollectionWeight float64
	CollectionName   string
}

type ScoredChunkID struct {
	ID    int64
	Score float64
	Rank  int64
}

type Query struct {
	Vector []float64
	Dim    int64
	Model  string
}

type RetrievalStrategy string

const (
	StrategyHybrid RetrievalStrategy = "hybrid"
	StrategyFTS    RetrievalStrategy = "fts"
	StrategyANN    RetrievalStrategy = "ann"
)
