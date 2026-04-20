package step

type ChunkToSave struct {
	Text     string
	Position int64
}

type ChunkResult struct {
	ChunksToSave           []ChunkToSave
	ParentRepresentationID int64
}

type ExtractionToChunk struct {
	ParentRepresentationID int64
	Nodes                  []ExtractionNode
}

type ExtractionNode struct {
	NodeType string
	Text     string
}
