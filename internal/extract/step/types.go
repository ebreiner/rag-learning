package step

type SourceDoc struct {
	SourcePath string
	RelPath    string
	Name       string
	SHA256     string
	SourceName string
	Additional map[string]string
}

type ExtractedDoc struct {
	MimeType  string
	Source    SourceDoc
	RootNodes []*Node
}
