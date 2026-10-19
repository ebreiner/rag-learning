package step

import "log/slog"

type RetrievalDeps struct {
	Logger           *slog.Logger
	Hydrator         ChunkHydrator
	Retriever        TopKRetriever
	EmbeddingsClient EmbedClient
	Renderer         ChunkRenderer
	Fusion           FusionConfig
	PoolSize         int64
	Sources          []string
}

type FusionConfig struct {
	RRFK      int64
	WeightFTS float64
	WeightANN float64
}

type RetrievedChunk struct {
	ID           int64
	Rank         int64
	DocTitle     string
	Position     int64
	Score        float64
	Breadcrumb   string
	Text         string
	SourceWeight float64
	SourceName   string
	Metadata     map[string]any
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
