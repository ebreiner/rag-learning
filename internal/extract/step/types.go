package step

type SourceDoc struct {
	SourcePath string
	Name       string
	SHA256     string
	Additional map[string]string
}

type ExtractedDoc struct {
	MimeType  string
	Source    SourceDoc
	RootNodes []*Node
}
