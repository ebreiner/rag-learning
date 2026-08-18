package step

import "context"

type ResultSink interface {
	SaveChunks(chunkResult ChunkResult, ctx context.Context) error
}

type ExtractionSource interface {
	NextExtraction(context.Context) (ExtractionToChunk, error)
}
