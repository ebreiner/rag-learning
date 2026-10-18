package step

import "time"

type Expectation struct {
	Doc                string `json:"doc"`
	BreadcrumbContains string `json:"breadcrumb_contains,omitempty"`
	TextContains       string `json:"text_contains,omitempty"`
}

type GoldenQuery struct {
	ID       string        `json:"id"`
	Query    string        `json:"query"`
	Expect   []Expectation `json:"expect"`
	Note     string        `json:"note"`
	NoAnswer bool          `json:"no_answer"`
}

type GoldenSet struct {
	Queries []GoldenQuery `json:"queries"`
}

// minimum data that the matcher needs
type Chunk struct {
	ID         int64  `json:"id"`
	DocName    string `json:"doc"`
	Breadcrumb string `json:"breadcrumb"`
	Text       string `json:"-"` // index text (chunks.text), not in the run file
}

type SearchHit struct {
	Chunk
	Score float64 `json:"score"`
}

type Status string

const (
	StatusHit         Status = "hit"
	StatusRankedOut   Status = "ranked_out"
	StatusContentLost Status = "content_lost"
	StatusDocMissing  Status = "doc_missing"
)

type QueryResult struct {
	QueryID string      `json:"query_id"`
	Status  Status      `json:"status,omitempty"` // empty for no_answer
	Rank    int         `json:"rank,omitempty"`
	DocRank int         `json:"doc_rank,omitempty"` // first rank with a chunk of an expected doc, 0 = none
	TopK    []SearchHit `json:"top_k"`
}

type Metrics struct {
	RecallAt     map[int]float64 `json:"recall_at"` // keys 1, 3, 5, 10
	MRR          float64         `json:"mrr"`
	DistinctSect float64         `json:"distinct_sections_at_10"`
	RankedOut    int             `json:"ranked_out"`
	ContentLost  int             `json:"content_lost"`
	DocMissing   int             `json:"doc_missing"`
	DocRecallAt  map[int]float64 `json:"doc_recall_at"`
	DocMRR       float64         `json:"doc_mrr"`
}

type StrategyResult struct {
	Strategy string        `json:"strategy"`
	Metrics  Metrics       `json:"metrics"`
	Queries  []QueryResult `json:"queries"`
}

type Run struct {
	Header     RunHeader        `json:"header"`
	Strategies []StrategyResult `json:"strategies"`
}

type RunHeader struct {
	Timestamp    time.Time `json:"timestamp"`
	GitCommit    string    `json:"git_commit"`
	GitDirty     bool      `json:"git_dirty"`
	GoldenPath   string    `json:"golden_path"`
	GoldenSHA256 string    `json:"golden_sha256"`
	DBPath       string    `json:"db_path"`
	ChunkCount   int64     `json:"chunk_count"`
	ChunksBuilt  string    `json:"chunks_built"` // newest chunks.created_at: shows a DB older than the chunk code
	EmbedModel   string    `json:"embed_model"`
	EmbedDim     int64     `json:"embed_dim"`
	K            int64     `json:"k"`
}

type StatusChange struct {
	QueryID  string
	Strategy string
	From     string
	To       string
}
