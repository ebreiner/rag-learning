package step

type SourceDoc struct {
	SourcePath string
	RelPath    string
	Name       string
	SHA256     string
	SourceName string
	Metadata   map[string]any
}

type ExtractedDoc struct {
	MimeType  string
	Source    SourceDoc
	RootNodes []*Node
}
