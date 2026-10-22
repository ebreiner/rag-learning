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
	ID           int64          `json:"id"`
	DocID        int64          `json:"doc_id"`
	Rank         int64          `json:"rank"`
	DocTitle     string         `json:"doc_title"`
	Position     int64          `json:"position"`
	Score        float64        `json:"score"`
	Breadcrumb   string         `json:"breadcrumb"`
	Text         string         `json:"text"`
	SourceWeight float64        `json:"source_weight"`
	SourceName   string         `json:"source_name"`
	Metadata     map[string]any `json:"-"`
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
