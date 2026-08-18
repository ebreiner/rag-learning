package step

import "context"

type DocSource interface {
	NextSourceDoc() (SourceDoc, error)
}

type Extractor interface {
	ExtractSourceDoc(doc SourceDoc, ctx context.Context) (ExtractedDoc, error)
}

type DocSink interface {
	SaveExtractedDoc(doc ExtractedDoc, ctx context.Context) (int64, error)
	ExistsDoc(sha256 string, ctx context.Context) (bool, error)
}
