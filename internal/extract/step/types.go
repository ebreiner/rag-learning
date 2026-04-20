package step

type SourceDoc struct {
	SourcePath string
	Name       string
	Additional map[string]string
}

type ExtractedMetadata struct {
	MimeType     string
	QualityScore float64
	Additional   []byte
}

type ExtractedDoc struct {
	Source   SourceDoc
	Metadata ExtractedMetadata
	Nodes    []Node
}

type Node struct {
	ID              string
	NodeType        string
	ParentIndex     *int64
	ChildrenIndexes *[]int64
	Page            *int64

	Level *int64
	Text  *string
}
