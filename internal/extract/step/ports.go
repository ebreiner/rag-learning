package step

import "context"

type DocSource interface {
	NextSourceDoc() (SourceDoc, error)
}

type Extractor interface {
	ExtractSourceDoc(ctx context.Context, doc SourceDoc) (ExtractedDoc, error)
}

type DocSink interface {
	SaveExtractedDoc(ctx context.Context, doc ExtractedDoc) (int64, error)
	ExistsDoc(ctx context.Context, sha256 string) (bool, error)
}
