package step

import "context"

type ResultSink interface {
	SaveChunks(ctx context.Context, chunkResult ChunkResult) error
}

type ExtractionSource interface {
	NextExtraction(context.Context) (ExtractionToChunk, error)
}
