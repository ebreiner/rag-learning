package step

import "context"

type Searcher interface {
	Search(ctx context.Context, query string, k int64, strategy string) ([]SearchHit, error)
}

type DocChunksProvider interface {
	ChunksOfDoc(ctx context.Context, docName string) (chunks []Chunk, exists bool, err error)
}
