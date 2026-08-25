package step

type SourceDoc struct {
	SourcePath       string
	Name             string
	SHA256           string
	CollectionName   string
	CollectionWeight float64
	Additional       map[string]string
}

type ExtractedDoc struct {
	MimeType  string
	Source    SourceDoc
	RootNodes []*Node
}
