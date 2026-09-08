package step

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var (
	testCtx    = context.Background()
	testLogger = slog.New(slog.DiscardHandler)
)

func mkHeadingNode(level int64, text string) *ExtractionNode {
	return &ExtractionNode{
		Kind:    "heading",
		Heading: &HeadingContent{Level: level, Text: text},
	}
}

func mkParagraphNode(text string) *ExtractionNode {
	return &ExtractionNode{
		Kind:      "paragraph",
		Paragraph: &ParagraphContent{Text: text},
	}
}

func withChildren(node *ExtractionNode, children ...*ExtractionNode) *ExtractionNode {
	node.Children = children
	return node
}

func mkTableNode(rows, cols int64, cells ...TableCell) *ExtractionNode {
	return &ExtractionNode{
		Kind:  "table",
		Table: &TableContent{Rows: rows, Cols: cols, Cells: cells},
	}
}

func mkListItemNode(id int64, marker, text string) *ExtractionNode {
	return &ExtractionNode{
		Kind:             KindListItem,
		ExtractionNodeID: id,
		List:             &ListItemContent{Marker: marker, Text: text},
	}
}

func TestWalk(t *testing.T) {
	tests := []struct {
		name    string
		root    *ExtractionNode
		want    []string // "breadcrumb|text" per candidate, in emitted order
		wantErr bool
	}{
		{
			name: "flat headings and paragraphs",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, "Intro"),
				mkParagraphNode("A"),
				mkHeadingNode(1, "Body"),
				mkParagraphNode("B"),
			),
			want: []string{"Intro|A", "Body|B"},
		},
		{
			name: "deeper heading pushes, same-or-shallower heading pops",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, "Ch1"),
				mkHeadingNode(2, "Sec1"),
				mkParagraphNode("A"),
				mkHeadingNode(2, "Sec2"),
				mkParagraphNode("B"),
			),
			want: []string{"Ch1 > Sec1|A", "Ch1 > Sec2|B"},
		},
		{
			name: "heading with nil content is skipped, does not corrupt breadcrumb",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, "Ch1"),
				&ExtractionNode{Kind: "heading", Heading: nil},
				mkParagraphNode("A"),
			),
			want: []string{"Ch1|A"},
		},
		{
			name: "heading with level <= 0 is skipped, does not corrupt breadcrumb",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, "Ch1"),
				mkHeadingNode(0, "Bad"),
				mkParagraphNode("A"),
			),
			want: []string{"Ch1|A"},
		},
		{
			name: "heading with missing title falls back to placeholder",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, ""),
				mkParagraphNode("A"),
			),
			want: []string{"MISSING_TITLE_PLACEHOLDER|A"},
		},
		{
			name: "unsupported node is skipped but its children are still visited",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "unsupported"},
					mkParagraphNode("A"),
				),
			),
			want: []string{"|A"},
		},
		{
			// Real trigger path: a docling "ordered_list" group is mapped to
			// unsupported, whose list_item children then surface at the top
			// level of the walk with no list parent to consume them. They must
			// be dropped, not abort the whole run, and siblings still walk.
			name: "orphan list_items under an unsupported parent are dropped, siblings still walk",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "unsupported"},
					mkListItemNode(1, "1.", "first"),
					mkListItemNode(2, "2.", "second"),
				),
				mkParagraphNode("after"),
			),
			want: []string{"|after"},
		},
		{
			name: "unknown kind returns an error",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				&ExtractionNode{Kind: "bogus"},
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := walk(testCtx, []*ExtractionNode{tt.root}, testLogger)
			if (err != nil) != tt.wantErr {
				t.Fatalf("walk() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			gotStrs := make([]string, len(got))
			for i, c := range got {
				gotStrs[i] = c.Breadcrumb + "|" + c.Text
			}

			if diff := cmp.Diff(tt.want, gotStrs, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("walk() candidates mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWalkTable(t *testing.T) {
	tests := []struct {
		name string
		root *ExtractionNode
		want []string // "breadcrumb|text" per candidate, in emitted order
	}{
		{
			// regression test: the header row (row 0) must never be rendered
			// as a data row, and every real data row (including the last
			// one) must be present. Guards against both the "starts at row
			// 0 instead of row 1" bug and the "loop stops one row early" bug.
			name: "renders the header row once and every data row in order",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkTableNode(3, 2,
					TableCell{Text: "Name", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 0, IsColumnHeader: true},
					TableCell{Text: "Price", RowStart: 0, RowEnd: 0, ColStart: 1, ColEnd: 1, IsColumnHeader: true},
					TableCell{Text: "Widget", RowStart: 1, RowEnd: 1, ColStart: 0, ColEnd: 0},
					TableCell{Text: "$10", RowStart: 1, RowEnd: 1, ColStart: 1, ColEnd: 1},
					TableCell{Text: "Gadget", RowStart: 2, RowEnd: 2, ColStart: 0, ColEnd: 0},
					TableCell{Text: "$20", RowStart: 2, RowEnd: 2, ColStart: 1, ColEnd: 1},
				),
			),
			want: []string{"|Name | Price\nWidget | $10\nGadget | $20\n"},
		},
		{
			name: "row-spanning cell is restated on every row it covers",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkTableNode(3, 3,
					TableCell{Text: "Category", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 0, IsColumnHeader: true},
					TableCell{Text: "Name", RowStart: 0, RowEnd: 0, ColStart: 1, ColEnd: 1, IsColumnHeader: true},
					TableCell{Text: "Price", RowStart: 0, RowEnd: 0, ColStart: 2, ColEnd: 2, IsColumnHeader: true},
					TableCell{Text: "Fruit", RowStart: 1, RowEnd: 2, ColStart: 0, ColEnd: 0},
					TableCell{Text: "Apple", RowStart: 1, RowEnd: 1, ColStart: 1, ColEnd: 1},
					TableCell{Text: "$1", RowStart: 1, RowEnd: 1, ColStart: 2, ColEnd: 2},
					TableCell{Text: "Banana", RowStart: 2, RowEnd: 2, ColStart: 1, ColEnd: 1},
					TableCell{Text: "$2", RowStart: 2, RowEnd: 2, ColStart: 2, ColEnd: 2},
				),
			),
			want: []string{"|Category | Name | Price\nFruit | Apple | $1\nFruit | Banana | $2\n"},
		},
		{
			name: "column-spanning header is applied to every column it covers",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkTableNode(2, 2,
					TableCell{Text: "Group", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 1, IsColumnHeader: true},
					TableCell{Text: "A", RowStart: 1, RowEnd: 1, ColStart: 0, ColEnd: 0},
					TableCell{Text: "B", RowStart: 1, RowEnd: 1, ColStart: 1, ColEnd: 1},
				),
			),
			want: []string{"|Group | Group\nA | B\n"},
		},
		{
			// Docling has no "number of header rows" field; header-ness is a
			// per-cell flag. A table with no header cells at all is a plain
			// grid where row 0 is data. Starting the data loop at a hardcoded
			// row 1 silently dropped that first row.
			name: "table without header cells emits row 0 as data",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkTableNode(2, 2,
					TableCell{Text: "A", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 0},
					TableCell{Text: "B", RowStart: 0, RowEnd: 0, ColStart: 1, ColEnd: 1},
					TableCell{Text: "C", RowStart: 1, RowEnd: 1, ColStart: 0, ColEnd: 0},
					TableCell{Text: "D", RowStart: 1, RowEnd: 1, ColStart: 1, ColEnd: 1},
				),
			),
			want: []string{"|\nA | B\nC | D\n"},
		},
		{
			// A grouped header occupies rows 0 and 1; data starts at row 2.
			// With the hardcoded row-1 start, row 1 rendered as an all-empty
			// data line. Known and accepted wart, documented here rather than
			// fixed: the per-column header map keeps only the last header cell
			// seen per column, so the "Register" group label is lost.
			name: "two-row header starts data at row 2 without a blank line",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkTableNode(3, 3,
					TableCell{Text: "Register", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 1, IsColumnHeader: true},
					TableCell{Text: "Bits", RowStart: 0, RowEnd: 1, ColStart: 2, ColEnd: 2, IsColumnHeader: true},
					TableCell{Text: "Lo", RowStart: 1, RowEnd: 1, ColStart: 0, ColEnd: 0, IsColumnHeader: true},
					TableCell{Text: "Hi", RowStart: 1, RowEnd: 1, ColStart: 1, ColEnd: 1, IsColumnHeader: true},
					TableCell{Text: "0x01", RowStart: 2, RowEnd: 2, ColStart: 0, ColEnd: 0},
					TableCell{Text: "0x02", RowStart: 2, RowEnd: 2, ColStart: 1, ColEnd: 1},
					TableCell{Text: "16", RowStart: 2, RowEnd: 2, ColStart: 2, ColEnd: 2},
				),
			),
			want: []string{"|Lo | Hi | Bits\n0x01 | 0x02 | 16\n"},
		},
		{
			name: "nil table content is skipped without panicking",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				&ExtractionNode{Kind: "table", Table: nil},
			),
			want: []string{},
		},
		{
			// documents current tolerant behavior for a malformed/empty
			// table (still emits one near-empty candidate rather than
			// skipping it, unlike list's "skip if nothing parsed" rule) --
			// worth revisiting deliberately, not silently.
			name: "table with no cells does not panic",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkTableNode(0, 0),
			),
			want: []string{"|\n"},
		},
		{
			name: "data cell in a column with no header degrades to an empty label",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkTableNode(2, 2,
					TableCell{Text: "Name", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 0, IsColumnHeader: true},
					TableCell{Text: "Widget", RowStart: 1, RowEnd: 1, ColStart: 0, ColEnd: 0},
					TableCell{Text: "$10", RowStart: 1, RowEnd: 1, ColStart: 1, ColEnd: 1},
				),
			),
			want: []string{"|Name | \nWidget | $10\n"},
		},
		{
			name: "table candidate carries the active breadcrumb",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, "Ch1"),
				mkTableNode(2, 2,
					TableCell{Text: "Name", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 0, IsColumnHeader: true},
					TableCell{Text: "Price", RowStart: 0, RowEnd: 0, ColStart: 1, ColEnd: 1, IsColumnHeader: true},
					TableCell{Text: "Widget", RowStart: 1, RowEnd: 1, ColStart: 0, ColEnd: 0},
					TableCell{Text: "$10", RowStart: 1, RowEnd: 1, ColStart: 1, ColEnd: 1},
				),
			),
			want: []string{"Ch1|Name | Price\nWidget | $10\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := walk(testCtx, []*ExtractionNode{tt.root}, testLogger)
			if err != nil {
				t.Fatalf("walk() error = %v", err)
			}

			gotStrs := make([]string, len(got))
			for i, c := range got {
				gotStrs[i] = c.Breadcrumb + "|" + c.Text
			}

			if diff := cmp.Diff(tt.want, gotStrs, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("walk() candidates mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestWalkList exercises the "list" case in walk(), including MemberIDs --
// the per-child node id list that lets mergeCandidates reference each
// list-item individually in chunk_nodes, instead of the container's own
// (content-less) node id.
// The table case must inline-consume its caption/footnote children into its
// own candidate (text appended, ids recorded after the table's own id) and
// must not let them fall through to the generic child push, where the
// walk's "not consumed by its parent" guard would drop them.
func TestWalkTableConsumesCaptionAndFootnote(t *testing.T) {
	table := mkTableNode(2, 1,
		TableCell{Text: "Name", RowStart: 0, RowEnd: 0, ColStart: 0, ColEnd: 0, IsColumnHeader: true},
		TableCell{Text: "Widget", RowStart: 1, RowEnd: 1, ColStart: 0, ColEnd: 0},
	)
	table.ExtractionNodeID = 200
	caption := &ExtractionNode{Kind: KindCaption, ExtractionNodeID: 300, Caption: &CaptionContent{Text: "Table 1: widgets"}}
	footnote := &ExtractionNode{Kind: KindFootnote, ExtractionNodeID: 400, Footnote: &FootnoteContent{Text: "1. prices excl. VAT"}}
	emptyCaption := &ExtractionNode{Kind: KindCaption, ExtractionNodeID: 500, Caption: &CaptionContent{Text: ""}}
	root := withChildren(&ExtractionNode{Kind: "unsupported"},
		withChildren(table, caption, footnote, emptyCaption),
	)

	got, err := walk(testCtx, []*ExtractionNode{root}, testLogger)
	if err != nil {
		t.Fatalf("walk() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d candidates, want exactly 1 (the table): %+v", len(got), got)
	}

	wantText := "Name\nWidget\n\nTable 1: widgets\n1. prices excl. VAT"
	if diff := cmp.Diff(wantText, got[0].Text); diff != "" {
		t.Errorf("table text mismatch (-want +got):\n%s", diff)
	}
	wantIDs := []int64{200, 300, 400}
	if diff := cmp.Diff(wantIDs, got[0].MemberIDs); diff != "" {
		t.Errorf("MemberIDs mismatch, empty caption must not be recorded (-want +got):\n%s", diff)
	}
}

func TestWalkList(t *testing.T) {
	type wantCandidate struct {
		breadcrumb string
		text       string
		memberIDs  []int64
	}

	tests := []struct {
		name string
		root *ExtractionNode
		want []wantCandidate
	}{
		{
			name: "list joins items with marker and a newline per item, and records each item's node id",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "list"},
					mkListItemNode(10, "-", "first"),
					mkListItemNode(11, "-", "second"),
				),
			),
			want: []wantCandidate{
				{breadcrumb: "", text: "- first\n- second\n", memberIDs: []int64{10, 11}},
			},
		},
		{
			name: "list item missing content is skipped, its id is not recorded, remaining items are unaffected",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "list"},
					mkListItemNode(1, "-", "keep"),
					&ExtractionNode{Kind: KindListItem, ExtractionNodeID: 2, List: nil},
					mkListItemNode(3, "-", "also keep"),
				),
			),
			want: []wantCandidate{
				{breadcrumb: "", text: "- keep\n- also keep\n", memberIDs: []int64{1, 3}},
			},
		},
		{
			// mirrors group's "if len(parts) > 0" guard: a list with nothing
			// usable inside it must not emit a stray empty-text candidate,
			// and walk() must still continue on to process later siblings.
			name: "list with no usable items produces no candidate, siblings still processed",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "list"},
					&ExtractionNode{Kind: KindListItem, ExtractionNodeID: 1, List: nil},
				),
				mkParagraphNode("after"),
			),
			want: []wantCandidate{
				{breadcrumb: "", text: "after"},
			},
		},
		{
			name: "list candidate carries the active breadcrumb",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, "Ch1"),
				withChildren(&ExtractionNode{Kind: "list"},
					mkListItemNode(5, "-", "x"),
				),
			),
			want: []wantCandidate{
				{breadcrumb: "Ch1", text: "- x\n", memberIDs: []int64{5}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := walk(testCtx, []*ExtractionNode{tt.root}, testLogger)
			if err != nil {
				t.Fatalf("walk() error = %v", err)
			}

			gotCandidates := make([]wantCandidate, len(got))
			for i, c := range got {
				gotCandidates[i] = wantCandidate{breadcrumb: c.Breadcrumb, text: c.Text, memberIDs: c.MemberIDs}
			}

			if diff := cmp.Diff(tt.want, gotCandidates, cmpopts.EquateEmpty(), cmp.AllowUnexported(wantCandidate{})); diff != "" {
				t.Errorf("walk() candidates mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestWalkGroup captures the design decisions for group-node support before
// it's implemented -- walk() has no `case "group"` yet, so every case here
// is expected to fail red (walk() currently returns "unknown node kind:
// group") until that case is added.
func TestWalkGroup(t *testing.T) {
	tests := []struct {
		name string
		root *ExtractionNode
		want []string // "breadcrumb|text" per candidate, in emitted order
	}{
		{
			// a group's children are fragments of ONE paragraph that docling
			// split at bold/italic span boundaries -- reassembling means
			// direct concatenation with NO separator, unlike list's
			// per-item "\n" or mergeCandidates' cross-candidate "\n\n".
			// The fragments themselves already carry any needed whitespace.
			name: "group concatenates its children's text with no separator",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "group"},
					mkParagraphNode("Please click "),
					mkParagraphNode("Save"),
					mkParagraphNode(" to continue."),
				),
			),
			want: []string{"|Please click Save to continue."},
		},
		{
			name: "non-paragraph child inside a group is skipped, does not corrupt the group text",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "group"},
					mkParagraphNode("before "),
					&ExtractionNode{Kind: "picture"},
					mkParagraphNode("after"),
				),
			),
			want: []string{"|before after"},
		},
		{
			// mirrors list's "if len(parts) > 0" guard: a group with nothing
			// usable inside it must not emit a stray empty-text candidate,
			// and walk() must still continue on to process later siblings.
			name: "group with no usable text produces no candidate, siblings still processed",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "group"},
					&ExtractionNode{Kind: "picture"},
					&ExtractionNode{Kind: "unsupported"},
				),
				mkParagraphNode("after"),
			),
			want: []string{"|after"},
		},
		{
			name: "group candidate carries the active breadcrumb",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				mkHeadingNode(1, "Ch1"),
				withChildren(&ExtractionNode{Kind: "group"},
					mkParagraphNode("A"),
					mkParagraphNode("B"),
				),
			),
			want: []string{"Ch1|A B"},
		},
		{
			// groups-in-groups: kept dumb on purpose (no recursion). A
			// nested group child must be skipped -- not silently treated as
			// empty/no-op text, and not recursed into -- while its own
			// siblings within the outer group are unaffected.
			name: "nested group child is skipped, not recursed into",
			root: withChildren(&ExtractionNode{Kind: "unsupported"},
				withChildren(&ExtractionNode{Kind: "group"},
					mkParagraphNode("before "),
					withChildren(&ExtractionNode{Kind: "group"},
						mkParagraphNode("nested"),
					),
					mkParagraphNode("after"),
				),
			),
			want: []string{"|before after"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := walk(testCtx, []*ExtractionNode{tt.root}, testLogger)
			if err != nil {
				t.Fatalf("walk() error = %v", err)
			}

			gotStrs := make([]string, len(got))
			for i, c := range got {
				gotStrs[i] = c.Breadcrumb + "|" + c.Text
			}

			if diff := cmp.Diff(tt.want, gotStrs, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("walk() candidates mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeCandidates(t *testing.T) {
	mkCandidate := func(breadcrumb, text string) chunkCandidate {
		return chunkCandidate{Node: mkParagraphNode(text), Breadcrumb: breadcrumb, Text: text}
	}
	mkIDCandidate := func(id int64, breadcrumb, text string) chunkCandidate {
		return chunkCandidate{
			Node:       &ExtractionNode{Kind: KindParagraph, ExtractionNodeID: id, Paragraph: &ParagraphContent{Text: text}},
			Breadcrumb: breadcrumb,
			Text:       text,
		}
	}
	mkContainerCandidate := func(kind NodeKind, id int64, breadcrumb, text string, memberIDs ...int64) chunkCandidate {
		return chunkCandidate{
			Node:       &ExtractionNode{Kind: kind, ExtractionNodeID: id},
			Breadcrumb: breadcrumb,
			Text:       text,
			MemberIDs:  memberIDs,
		}
	}

	tests := []struct {
		name       string
		candidates []chunkCandidate
		want       []ChunkToSave
	}{
		{
			name:       "empty input produces no chunks",
			candidates: nil,
			want:       nil,
		},
		{
			name: "same breadcrumb merges into one chunk",
			candidates: []chunkCandidate{
				mkCandidate("Ch1", "A"),
				mkCandidate("Ch1", "B"),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\nA\n\nB", Breadcrumb: "Ch1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{}, {Position: 1}}},
			},
		},
		{
			name: "breadcrumb change forces a cut even under budget",
			candidates: []chunkCandidate{
				mkCandidate("Ch1", "A"),
				mkCandidate("Ch2", "B"),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\nA", Breadcrumb: "Ch1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{}}},
				{Text: "Ch2\n\nB", Breadcrumb: "Ch2", Position: 1, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{Position: 1}}},
			},
		},
		{
			name: "budget overflow forces a cut within the same breadcrumb",
			candidates: []chunkCandidate{
				mkCandidate("Ch1", strings.Repeat("a", 600)),
				mkCandidate("Ch1", strings.Repeat("b", 600)),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\n" + strings.Repeat("a", 600), Breadcrumb: "Ch1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{}}},
				{Text: "Ch1\n\n" + strings.Repeat("b", 600), Breadcrumb: "Ch1", Position: 1, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{Position: 1}}},
			},
		},
		{
			// regression test: a candidate that overflows the budget must
			// seed the next chunk, not be silently dropped.
			name: "overflowing candidate is not dropped",
			candidates: []chunkCandidate{
				mkCandidate("Ch1", strings.Repeat("a", 900)),
				mkCandidate("Ch1", strings.Repeat("b", 200)),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\n" + strings.Repeat("a", 900), Breadcrumb: "Ch1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{}}},
				{Text: "Ch1\n\n" + strings.Repeat("b", 200), Breadcrumb: "Ch1", Position: 1, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{Position: 1}}},
			},
		},
		{
			// regression test: a single candidate whose own text already
			// exceeds maxBudget must still be emitted -- specifically when it
			// arrives with an empty buffer (e.g. right after a flush), where
			// neither the "fits in budget" branch nor the "flush what's
			// buffered so far" branch fires. Distinct from "overflowing
			// candidate is not dropped" above, which never actually exercises
			// a candidate that's oversized on its own -- both of its
			// candidates individually fit under 1000 chars.
			name: "candidate larger than maxBudget alone is not dropped when buffer is empty",
			candidates: []chunkCandidate{
				mkCandidate("Ch1", strings.Repeat("a", 1200)),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\n" + strings.Repeat("a", 1200), Breadcrumb: "Ch1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{}}},
			},
		},
		{
			name: "nil paragraph content is skipped without panicking",
			candidates: []chunkCandidate{
				{Node: &ExtractionNode{Kind: "paragraph", Paragraph: nil}, Breadcrumb: "Ch1"},
				mkCandidate("Ch1", "A"),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\nA", Breadcrumb: "Ch1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{}, {Position: 1}}},
			},
		},
		// The next three cases are expected to fail red: mergeCandidates'
		// table/group/list branches stamp the emitted ChunkToSave's
		// Breadcrumb from `lastCrumb` (the content-merge buffer's carried-
		// over state), not from the container candidate's own (already
		// correct, walk()-assigned) Breadcrumb field. That's indistinguishable
		// from correct whenever a container happens to be preceded by a
		// content candidate under the same breadcrumb (lastCrumb already
		// matches by coincidence -- see the splice case below), but is wrong
		// standalone: a table/list/group with no preceding content candidate
		// under its heading -- including one that opens a document -- gets
		// stamped with a stale or empty breadcrumb instead of its own. Real
		// bug, not a test mistake; left failing on purpose rather than
		// asserting the wrong value.
		{
			name: "table candidate is emitted standalone, tagged, and referenced by its own node id",
			candidates: []chunkCandidate{
				// walk() always seeds a table candidate's MemberIDs with the
				// table's own node id first (position 0), then appends any
				// consumed caption/footnote children after it.
				mkContainerCandidate(KindTable, 200, "Ch1", "header | row\n", 200),
			},
			want: []ChunkToSave{
				{Text: "header | row\n", Breadcrumb: "Ch1", Position: 0, Type: TypeTable,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 200}}},
			},
		},
		{
			// a table with a consumed caption: MemberIDs is [table's own id,
			// caption's id] (walk() seeds its own id first, then appends the
			// caption it inlined) -- both must end up referenced, table's own
			// id still at position 0.
			name: "table candidate referencing its own node id plus a consumed caption's node id",
			candidates: []chunkCandidate{
				mkContainerCandidate(KindTable, 200, "Ch1", "header | row\nFigure 1: a caption", 200, 300),
			},
			want: []ChunkToSave{
				{Text: "header | row\nFigure 1: a caption", Breadcrumb: "Ch1", Position: 0, Type: TypeTable,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 200}, {ExtractionNodeID: 300, Position: 1}}},
			},
		},
		{
			// group's own node id must never appear in ExtractionNodeIDs --
			// only its members', since the group container itself carries no
			// content_json (marshalContent has no case for it).
			name: "group candidate is referenced by its members' node ids, not its own",
			candidates: []chunkCandidate{
				mkContainerCandidate(KindGroup, 300, "Ch1", "AB", 10, 11),
			},
			want: []ChunkToSave{
				{Text: "AB", Breadcrumb: "Ch1", Position: 0, Type: TypeGeneric,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 10}, {ExtractionNodeID: 11, Position: 1}}},
			},
		},
		{
			name: "list candidate is referenced by its members' node ids, not its own",
			candidates: []chunkCandidate{
				mkContainerCandidate(KindList, 400, "Ch1", "- a\n- b\n", 20, 21),
			},
			want: []ChunkToSave{
				{Text: "- a\n- b\n", Breadcrumb: "Ch1", Position: 0, Type: TypeList,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 20}, {ExtractionNodeID: 21, Position: 1}}},
			},
		},
		{
			name: "a container candidate with empty text is dropped, not emitted as an empty chunk",
			candidates: []chunkCandidate{
				mkContainerCandidate(KindGroup, 300, "Ch1", ""),
			},
			want: nil,
		},
		{
			name: "an unsupported candidate is dropped and never reaches the output",
			candidates: []chunkCandidate{
				{Node: &ExtractionNode{Kind: KindUnsupported, ExtractionNodeID: 999}, Breadcrumb: "Ch1", Text: "ignored"},
			},
			want: nil,
		},
		{
			// regression test: a container candidate spliced between two
			// content candidates under the same breadcrumb must neither flush
			// nor split the surrounding buffer -- the content on both sides
			// merges into one chunk, with the container emitted as its own
			// separate chunk alongside it, not blocking or duplicating
			// position numbers.
			name: "a container candidate spliced between content candidates does not disturb the surrounding merge",
			candidates: []chunkCandidate{
				mkIDCandidate(100, "Ch1", "A"),
				mkContainerCandidate(KindTable, 200, "Ch1", "TABLE", 200),
				mkIDCandidate(101, "Ch1", "B"),
			},
			want: []ChunkToSave{
				{Text: "TABLE", Breadcrumb: "Ch1", Position: 0, Type: TypeTable,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 200}}},
				{Text: "Ch1\n\nA\n\nB", Breadcrumb: "Ch1", Position: 1, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 100}, {ExtractionNodeID: 101, Position: 1}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeCandidates(testCtx, tt.candidates, testLogger)
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mergeCandidates() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
