package types

type Node struct {
	ID                string `json:"id"`
	NodeType          string `json:"node_type"`
	Parent            int    `json:"parent"`
	Children          []int  `json:"children,omitempty"`
	GroupHeadingLevel int    `json:"group_heading_level,omitempty"`
	GroupHeadingText  string `json:"group_heading_text,omitempty"`
	Level             int    `json:"level"`
	Text              string `json:"text"`
	Page              int    `json:"page,omitempty"`
}

type Document struct {
	Title        string   `json:"title"`
	Nodes        []Node   `json:"nodes,omitempty"`
	SourceFormat string   `json:"source_format"`
	MimeType     string   `json:"mime_type"`
	Metadata     MetaData `json:"metadata"`
	Chunks       []Chunk  `json:"chunks,omitempty"`
}

type MetaData struct {
	QualityScore float64  `json:"quality_score,omitempty"`
	Mail         MailData `json:"mail"`
}

type MailData struct {
	Subject  string   `json:"subject,omitempty"`
	MailFrom string   `json:"mail_from,omitempty"`
	MailCC   []string `json:"mail_cc,omitempty"`
	MailTo   []string `json:"mail_to,omitempty"`
}

type Chunk struct {
	Text       string    `json:"text"`
	ChunkCount int       `json:"chunk_count"`
	ChunkIndex int       `json:"chunk_index"`
	Embedding  []float64 `json:"embedding"`
}
