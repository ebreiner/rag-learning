package step

type NodeKind string

const (
	KindHeading     NodeKind = "heading"
	KindParagraph   NodeKind = "paragraph"
	KindCode        NodeKind = "code"
	KindCaption     NodeKind = "caption"
	KindFootnote    NodeKind = "footnote"
	KindFormula     NodeKind = "formula"
	KindListItem    NodeKind = "list_item"
	KindList        NodeKind = "list"
	KindTable       NodeKind = "table"
	KindPicture     NodeKind = "picture"
	KindGroup       NodeKind = "group" // generic container
	KindUnsupported NodeKind = "unsupported"
)

type ContentLayer string

const (
	LayerBody       ContentLayer = "body"
	LayerFurniture  ContentLayer = "furniture" // header/footer/page boilerplate
	LayerBackground ContentLayer = "background"
	LayerInvisible  ContentLayer = "invisible"
	LayerNotes      ContentLayer = "notes"
)

type Node struct {
	ID       string
	Kind     NodeKind
	Layer    ContentLayer
	Parent   *Node `json:"-"` // excluded: children always reach down and we dont like cyclic dependencies in json land
	Children []*Node

	Provenance []Provenance // one per page/fragment this node spans

	Heading   *HeadingContent
	Paragraph *ParagraphContent
	Code      *CodeContent
	Formula   *FormulaContent
	Caption   *ParagraphContent // same shape as paragraph
	Footnote  *ParagraphContent
	ListItem  *ListItemContent
	Table     *TableContent
	Picture   *PictureContent
}

type Provenance struct {
	Page     int64
	BBox     *BBox     // optional not every adapter can supply this
	CharSpan *CharSpan // optional nil means not applicable, distinct from a zero-length span
}

// Always normalized to top-left origin (reminder: y increseases downward)
// Adapter must ensure this
type BBox struct {
	Left, Top, Right, Bottom float64
}

// CharSpan half open into nodes text
type CharSpan struct {
	Start, End int64
}

type HeadingContent struct {
	Text  string
	Level int64
}
type ParagraphContent struct{ Text string }

type CodeContent struct{ Text string }
type FormulaContent struct{ Text string }

type ListItemContent struct {
	Text       string
	Enumerated bool
	Marker     string
}
type TableContent struct {
	Rows, Cols int64
	Cells      []TableCell
}
type TableCell struct {
	Text           string
	BBox           *BBox // optional not every adapter can supply per-cell bbox
	RowStart       int64
	RowEnd         int64
	ColStart       int64
	ColEnd         int64
	IsColumnHeader bool
	IsRowHeader    bool
	RowSection     bool
	Fillable       bool
}

type PictureContent struct {
	Annotations []string      // generated, TODO: empirical data needed
	Image       *PictureImage // nil if the source couldn't supply image data
}

type PictureImage struct {
	MimeType      string
	Data          []byte
	Width, Height float64
}
