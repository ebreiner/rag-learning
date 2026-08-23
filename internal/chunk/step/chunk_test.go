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

func TestMergeCandidates(t *testing.T) {
	mkCandidate := func(breadcrumb, text string) chunkCandidate {
		return chunkCandidate{Node: mkParagraphNode(text), Breadcrumb: breadcrumb, Text: text}
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
				{Text: "Ch1\n\nA\n\nB", Breadcrumb: "Ch1", Position: 0},
			},
		},
		{
			name: "breadcrumb change forces a cut even under budget",
			candidates: []chunkCandidate{
				mkCandidate("Ch1", "A"),
				mkCandidate("Ch2", "B"),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\nA", Breadcrumb: "Ch1", Position: 0},
				{Text: "Ch2\n\nB", Breadcrumb: "Ch2", Position: 1},
			},
		},
		{
			name: "budget overflow forces a cut within the same breadcrumb",
			candidates: []chunkCandidate{
				mkCandidate("Ch1", strings.Repeat("a", 600)),
				mkCandidate("Ch1", strings.Repeat("b", 600)),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\n" + strings.Repeat("a", 600), Breadcrumb: "Ch1", Position: 0},
				{Text: "Ch1\n\n" + strings.Repeat("b", 600), Breadcrumb: "Ch1", Position: 1},
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
				{Text: "Ch1\n\n" + strings.Repeat("a", 900), Breadcrumb: "Ch1", Position: 0},
				{Text: "Ch1\n\n" + strings.Repeat("b", 200), Breadcrumb: "Ch1", Position: 1},
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
				{Text: "Ch1\n\n" + strings.Repeat("a", 1200), Breadcrumb: "Ch1", Position: 0},
			},
		},
		{
			name: "nil paragraph content is skipped without panicking",
			candidates: []chunkCandidate{
				{Node: &ExtractionNode{Kind: "paragraph", Paragraph: nil}, Breadcrumb: "Ch1"},
				mkCandidate("Ch1", "A"),
			},
			want: []ChunkToSave{
				{Text: "Ch1\n\nA", Breadcrumb: "Ch1", Position: 0},
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
