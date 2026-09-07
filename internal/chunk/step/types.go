package step

type ChunkToSave struct {
	Text              string
	Breadcrumb        string
	Position          int64
	ExtractionNodeIDs []ChunkExtractionNodeID
	Type              ChunkType
}

type ChunkExtractionNodeID struct {
	ExtractionNodeID int64
	Position         int64
}

type ChunkResult struct {
	ChunksToSave []ChunkToSave
	DocumentID   int64
}

type ExtractionToChunk struct {
	DocumentID int64
	Roots      []*ExtractionNode
}

type ChunkType string

const (
	TypeGeneric ChunkType = "generic"
	TypeContent ChunkType = "content"
	TypeCode    ChunkType = "code"
	TypeFormula ChunkType = "formula"
	TypeList    ChunkType = "list"
	TypeTable   ChunkType = "table"
)

type NodeKind string

const (
	KindHeading     NodeKind = "heading"
	KindParagraph   NodeKind = "paragraph"
	KindCaption     NodeKind = "caption"
	KindCode        NodeKind = "code"
	KindFormula     NodeKind = "formula"
	KindFootnote    NodeKind = "footnote"
	KindListItem    NodeKind = "list_item"
	KindList        NodeKind = "list"
	KindTable       NodeKind = "table"
	KindPicture     NodeKind = "picture"
	KindGroup       NodeKind = "group" // generic container
	KindUnsupported NodeKind = "unsupported"
)

type ContentLayer string

const (
	LayerFurniture ContentLayer = "furniture"
	LayerBody      ContentLayer = "body"
)

type ExtractionNode struct {
	ID               string
	ExtractionNodeID int64
	Kind             NodeKind
	Parent           *ExtractionNode
	Layer            ContentLayer
	Children         []*ExtractionNode
	Paragraph        *ParagraphContent
	Code             *CodeContent
	Formula          *FormulaContent
	Caption          *CaptionContent
	Footnote         *FootnoteContent
	Heading          *HeadingContent
	List             *ListItemContent
	Table            *TableContent
}

type TableContent struct {
	Rows  int64
	Cols  int64
	Cells []TableCell
}

type TableCell struct {
	Text           string
	RowStart       int64
	RowEnd         int64
	ColStart       int64
	ColEnd         int64
	IsColumnHeader bool
}

type HeadingContent struct {
	Level int64  `json:"level"`
	Text  string `json:"text"`
}

type ParagraphContent struct {
	Text string `json:"text"`
}

type CaptionContent struct {
	Text string `json:"text"`
}

type FootnoteContent struct {
	Text string `json:"text"`
}

type FormulaContent struct {
	Text string `json:"text"`
}

type CodeContent struct {
	Text string `json:"text"`
}

type ListItemContent struct {
	Text       string `json:"text"`
	Marker     string `json:"marker"`
	Enumerated bool   `json:"enumerated"`
}
