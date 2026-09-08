package docling

// Top-level response from POST /v1/convert/file
type rawConvertResponse struct {
	Document       rawDoclingWrapper `json:"document"`
	Errors         []any             `json:"errors"`
	ProcessingTime float64           `json:"processing_time"`
	Status         string            `json:"status"`
}

// document.* — only json_content matters if you requested to_formats=json;
// md_content/html_content/etc. are sibling export formats, safe to ignore
// (encoding/json silently drops fields you don't declare).
type rawDoclingWrapper struct {
	Filename    string              `json:"filename"`
	JSONContent *rawDoclingDocument `json:"json_content"`
}

// The DoclingDocument itself.
type rawDoclingDocument struct {
	SchemaName string             `json:"schema_name"`
	Version    string             `json:"version"`
	Name       string             `json:"name"`
	Origin     rawOrigin          `json:"origin"`
	Body       rawDoclingNodeRef  `json:"body"`
	Furniture  rawDoclingNodeRef  `json:"furniture"`
	Groups     []rawGroupItem     `json:"groups"`
	Texts      []rawTextItem      `json:"texts"`
	Pictures   []rawPictureItem   `json:"pictures"`
	Tables     []rawTableItem     `json:"tables"`
	Pages      map[string]rawPage `json:"pages"`

	// Declared by the schema but never populated by any current Docling
	// layout model — the model detects "Form"/"Key-Value Region" regions
	// but downstream processing currently ignores both. Kept as []any so
	// nothing is silently dropped if/when this ships.
	KeyValueItems []any `json:"key_value_items"`
	FormItems     []any `json:"form_items"`
}

// document.origin — source file identity, incl. Docling's own content hash.
type rawOrigin struct {
	MimeType   string  `json:"mimetype"`
	BinaryHash uint64  `json:"binary_hash"`
	Filename   string  `json:"filename"`
	URI        *string `json:"uri"`
}

// $ref pointer, e.g. {"$ref": "#/texts/3"} — appears as `parent` and inside `children`.
type rawRef struct {
	Ref string `json:"$ref"`
}

// body/furniture roots and group nodes share this shape — no `text`, just structure.
// NOTE: `furniture` (self_ref "#/furniture") is not a separate tree in practice —
// confirmed live: furniture.children is always empty, and furniture-layer items
// (page_header/page_footer) have parent #/body like everything else. ContentLayer
// is a per-item field inside the one body tree, not a structural split.
type rawDoclingNodeRef struct {
	SelfRef      string   `json:"self_ref"`
	Parent       *rawRef  `json:"parent"`
	Children     []rawRef `json:"children"`
	ContentLayer string   `json:"content_layer"`
	Name         string   `json:"name"`
	Label        string   `json:"label"`
}

type rawGroupItem struct {
	rawDoclingNodeRef
	// groups today only carry the shared fields above (name="list", label="list").
}

// texts[] covers section_header / text / list_item / caption / footnote /
// page_header / page_footer — Docling puts all of these in one array,
// differentiated by Label. Captions/footnotes reached via a picture's or

// Captions/footnotes are entries in this array with Parent "#/body"; the
// host picture/table references them through its own captions/footnotes
// refs, wireGraph re-parents them under the host
type rawTextItem struct {
	SelfRef      string         `json:"self_ref"`
	Parent       *rawRef        `json:"parent"`
	Children     []rawRef       `json:"children"`
	ContentLayer string         `json:"content_layer"`
	Label        string         `json:"label"` // "section_header" | "text" | "list_item" | "caption" | "footnote" | "page_header" | "page_footer"
	Prov         []rawProv      `json:"prov"`
	Orig         string         `json:"orig"`
	Text         string         `json:"text"`
	Formatting   *rawFormatting `json:"formatting"`
	Hyperlink    *string        `json:"hyperlink"`

	Level      *int64  `json:"level"`      // section_header only
	Enumerated *bool   `json:"enumerated"` // list_item only
	Marker     *string `json:"marker"`     // list_item only
}

type rawPictureItem struct {
	SelfRef      string    `json:"self_ref"`
	Parent       *rawRef   `json:"parent"`
	Children     []rawRef  `json:"children"`
	ContentLayer string    `json:"content_layer"`
	Label        string    `json:"label"`
	Prov         []rawProv `json:"prov"`
	Captions     []rawRef  `json:"captions"` // refs into texts[] — same items as Children, kept as a typed convenience view
	References   []rawRef  `json:"references"`
	Footnotes    []rawRef  `json:"footnotes"`
	Image        *rawImage `json:"image"`
	Annotations  []any     `json:"annotations"` // populated only if picture-description
}

type rawImage struct {
	MimeType string  `json:"mimetype"`
	DPI      int64   `json:"dpi"`
	Size     rawSize `json:"size"`
	URI      string  `json:"uri"` // data: URI, base64
}

type rawSize struct {
	Width, Height float64
}

// Confirmed live against a doc with 5 real tables
type rawTableItem struct {
	SelfRef      string       `json:"self_ref"`
	Parent       *rawRef      `json:"parent"`
	Children     []rawRef     `json:"children"`
	ContentLayer string       `json:"content_layer"`
	Label        string       `json:"label"`
	Prov         []rawProv    `json:"prov"`
	Data         rawTableData `json:"data"`
	Captions     []rawRef     `json:"captions"`
	Footnotes    []rawRef     `json:"footnotes"`
}

type rawTableData struct {
	NumRows int64          `json:"num_rows"`
	NumCols int64          `json:"num_cols"`
	Cells   []rawTableCell `json:"table_cells"`
}

type rawTableCell struct {
	BBox              rawBBox `json:"bbox"` // each cell carries its own bbox, distinct from the table's overall Prov bbox
	Text              string  `json:"text"`
	RowSpan           int64   `json:"row_span"`
	ColSpan           int64   `json:"col_span"`
	StartRowOffsetIdx int64   `json:"start_row_offset_idx"`
	EndRowOffsetIdx   int64   `json:"end_row_offset_idx"`
	StartColOffsetIdx int64   `json:"start_col_offset_idx"`
	EndColOffsetIdx   int64   `json:"end_col_offset_idx"`
	ColumnHeader      bool    `json:"column_header"`
	RowHeader         bool    `json:"row_header"`
	RowSection        bool    `json:"row_section"`
	Fillable          bool    `json:"fillable"`
}

type rawProv struct {
	PageNo   int64    `json:"page_no"`
	BBox     rawBBox  `json:"bbox"`
	CharSpan [2]int64 `json:"charspan"` // [start, end)
}

type rawBBox struct {
	Left        float64 `json:"l"`
	Top         float64 `json:"t"`
	Right       float64 `json:"r"`
	Bottom      float64 `json:"b"`
	CoordOrigin string  `json:"coord_origin"` // "BOTTOMLEFT" for page-level prov, "TOPLEFT" seen on table cells — do not assume one origin codebase-wide
}

type rawFormatting struct {
	Bold      bool `json:"bold"`
	Italic    bool `json:"italic"`
	Underline bool `json:"underline"`
}

type rawPage struct {
	Size  rawSize       `json:"size"`
	Image *rawPageImage `json:"image"`
}

type rawPageImage struct {
	MimeType string  `json:"mimetype"`
	DPI      int64   `json:"dpi"`
	Size     rawSize `json:"size"`
	URI      string  `json:"uri"`
}
