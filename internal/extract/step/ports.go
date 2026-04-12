package step

type DocSource interface {
	NextSourceDoc() (SourceDoc, error)
}

type Extractor interface {
	ExtractSourceDoc(doc SourceDoc) (ExtractedDoc, error)
}

type DocSink interface {
	SaveExtractedDoc(doc ExtractedDoc) error
}
