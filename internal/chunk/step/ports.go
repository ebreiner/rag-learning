package step

import ()

type ResultSink interface {
	SaveChunks(chunkResult ChunkResult) error
}

type ExtractionSource interface {
	NextExtraction() (ExtractionToChunk, error)
}
