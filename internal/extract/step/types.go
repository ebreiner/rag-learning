package step

type SourceDoc struct {
	SourcePath string
	Name       string
	SHA256     string
	Additional map[string]string
}

type ExtractedMetadata struct {
	MimeType     string
	QualityScore float64
	Additional   []byte
}

type ExtractedDoc struct {
	Source    SourceDoc
	Metadata  ExtractedMetadata
	RootNodes []*Node
}
