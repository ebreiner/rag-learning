package step

type SourceDoc struct {
	SourcePath string
	RelPath    string
	Name       string
	SHA256     string
	SourceName string
	Metadata   map[string]any
	Links      []Link
}

type Link struct {
	To      string
	Text    string
	Section string
}

type ExtractedDoc struct {
	MimeType  string
	Source    SourceDoc
	RootNodes []*Node
}
