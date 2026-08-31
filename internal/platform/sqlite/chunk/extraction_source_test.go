package chunk

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/querries"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

var (
	testCtx    = context.Background()
	testLogger = slog.New(slog.DiscardHandler)
)

func ns(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func mkRow(nodeID, parentID, kind, layer, contentJSON string) querries.GetLatestExtractionOfDocRow {
	return querries.GetLatestExtractionOfDocRow{
		NodeID:      nodeID,
		ParentID:    ns(parentID),
		Kind:        kind,
		Layer:       layer,
		ContentJson: ns(contentJSON),
	}
}

func nodeRow(nodeID, parentID string) querries.GetLatestExtractionOfDocRow {
	return querries.GetLatestExtractionOfDocRow{NodeID: nodeID, ParentID: ns(parentID)}
}

func bareNode(id string) *step.ExtractionNode {
	return &step.ExtractionNode{ID: id}
}

// assertParentLinks checks Parent/Children are mutually consistent with the
// parent_id relationships implied by rows, including that Children preserves
// row order. Deliberately does not deep-compare Parent (cyclic); it checks
// pointer identity against nodes instead.
func assertParentLinks(t *testing.T, nodes map[string]*step.ExtractionNode, rows []querries.GetLatestExtractionOfDocRow) {
	t.Helper()

	wantChildren := make(map[string][]string)
	for _, r := range rows {
		node, ok := nodes[r.NodeID]
		if !ok {
			continue
		}
		if !r.ParentID.Valid {
			if node.Parent != nil {
				t.Errorf("node %s: expected nil Parent for root node, got %v", node.ID, node.Parent.ID)
			}
			continue
		}
		wantParent, ok := nodes[r.ParentID.String]
		if !ok {
			continue
		}
		if node.Parent != wantParent {
			t.Errorf("node %s: Parent = %v, want %v", node.ID, node.Parent, wantParent.ID)
		}
		wantChildren[r.ParentID.String] = append(wantChildren[r.ParentID.String], node.ID)
	}

	for parentID, want := range wantChildren {
		parent := nodes[parentID]
		got := make([]string, len(parent.Children))
		for i, c := range parent.Children {
			got[i] = c.ID
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("node %s: Children mismatch (-want +got):\n%s", parentID, diff)
		}
	}
}

type buildMapCase struct {
	name    string
	rows    []querries.GetLatestExtractionOfDocRow
	want    map[string]*step.ExtractionNode
	wantErr bool
}

func TestBuildMap(t *testing.T) {
	cases := []buildMapCase{
		{
			name: "heading with content",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "heading", "body", `{"level":1,"text":"Intro"}`),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindHeading, Layer: step.LayerBody,
					Heading: &step.HeadingContent{Level: 1, Text: "Intro"}},
			},
		},
		{
			name: "heading without content is not an error",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "heading", "body", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindHeading, Layer: step.LayerBody},
			},
		},
		{
			name: "paragraph with content",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/1", "#/texts/0", "paragraph", "body", `{"text":"hello world"}`),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/1": {ID: "#/texts/1", Kind: step.KindParagraph, Layer: step.LayerBody,
					Paragraph: &step.ParagraphContent{Text: "hello world"}},
			},
		},
		{
			name: "paragraph without content is not an error",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/1", "#/texts/0", "paragraph", "body", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/1": {ID: "#/texts/1", Kind: step.KindParagraph, Layer: step.LayerBody},
			},
		},
		{
			name: "list maps to KindList",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "list", "body", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindList, Layer: step.LayerBody},
			},
		},
		{
			name: "list_item with content",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/1", "#/texts/0", "list_item", "body", `{"text":"first item","marker":"-","enumerated":false}`),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/1": {ID: "#/texts/1", Kind: step.KindListItem, Layer: step.LayerBody,
					List: &step.ListItemContent{Text: "first item", Marker: "-", Enumerated: false}},
			},
		},
		{
			name: "list_item without content is not an error",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/1", "#/texts/0", "list_item", "body", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/1": {ID: "#/texts/1", Kind: step.KindListItem, Layer: step.LayerBody},
			},
		},
		{
			name: "malformed content json errors",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "heading", "body", `{"level":`),
			},
			wantErr: true,
		},
		{
			// regression test: node.ExtractionNodeID must come from the row's
			// ExtractionNodeID column (en.id, the per-node surrogate PK), not
			// from ExtractionID (e.id, the extraction row's PK -- shared by
			// every node in the same document). Uses distinct, deliberately
			// mismatched values for the two so a mixup produces a visibly
			// wrong number instead of accidentally passing.
			name: "ExtractionNodeID comes from the row's per-node id, not the shared extraction id",
			rows: []querries.GetLatestExtractionOfDocRow{
				{ExtractionID: 1, ExtractionNodeID: 42, NodeID: "#/texts/0", Kind: "paragraph", Layer: "body"},
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", ExtractionNodeID: 42, Kind: step.KindParagraph, Layer: step.LayerBody},
			},
		},
		{
			name: "caption maps to KindCaption, content unmarshals into Paragraph (shared shape)",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "caption", "body", `{"text":"Figure 1: a caption"}`),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindCaption, Layer: step.LayerBody,
					Paragraph: &step.ParagraphContent{Text: "Figure 1: a caption"}},
			},
		},
		{
			name: "caption without content is not an error",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "caption", "body", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindCaption, Layer: step.LayerBody},
			},
		},
		{
			name: "footnote maps to KindFootnote, content unmarshals into Paragraph (shared shape)",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "footnote", "body", `{"text":"1. see appendix"}`),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindFootnote, Layer: step.LayerBody,
					Paragraph: &step.ParagraphContent{Text: "1. see appendix"}},
			},
		},
		{
			name: "footnote without content is not an error",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "footnote", "body", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindFootnote, Layer: step.LayerBody},
			},
		},
		{
			name: "furniture layer",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "paragraph", "furniture", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindParagraph, Layer: step.LayerFurniture},
			},
		},
		{
			name: "unknown layer errors",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "paragraph", "bogus", ""),
			},
			wantErr: true,
		},
		{
			name: "unknown kind errors",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", "bogus_kind", "body", ""),
			},
			wantErr: true,
		},
	}

	for _, kind := range []string{"unsupported", "picture"} {
		cases = append(cases, buildMapCase{
			name: kind + " maps to KindUnsupported",
			rows: []querries.GetLatestExtractionOfDocRow{
				mkRow("#/texts/0", "", kind, "body", ""),
			},
			want: map[string]*step.ExtractionNode{
				"#/texts/0": {ID: "#/texts/0", Kind: step.KindUnsupported, Layer: step.LayerBody},
			},
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildMap(tc.rows, testCtx, testLogger)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("buildMap() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildMap() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("buildMap() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

type wireGraphCase struct {
	name    string
	nodes   map[string]*step.ExtractionNode
	rows    []querries.GetLatestExtractionOfDocRow
	wantIDs []string
	wantErr bool
}

func TestWireGraph(t *testing.T) {
	cases := []wireGraphCase{
		{
			name:    "single root, no children",
			nodes:   map[string]*step.ExtractionNode{"A": bareNode("A")},
			rows:    []querries.GetLatestExtractionOfDocRow{nodeRow("A", "")},
			wantIDs: []string{"A"},
		},
		{
			name: "parent with one child",
			nodes: map[string]*step.ExtractionNode{
				"A": bareNode("A"), "B": bareNode("B"),
			},
			rows: []querries.GetLatestExtractionOfDocRow{
				nodeRow("A", ""), nodeRow("B", "A"),
			},
			wantIDs: []string{"A"},
		},
		{
			name: "multiple children preserve row order",
			nodes: map[string]*step.ExtractionNode{
				"A": bareNode("A"), "B": bareNode("B"), "C": bareNode("C"), "D": bareNode("D"),
			},
			rows: []querries.GetLatestExtractionOfDocRow{
				nodeRow("A", ""), nodeRow("B", "A"), nodeRow("C", "A"), nodeRow("D", "A"),
			},
			wantIDs: []string{"A"},
		},
		{
			name: "multi-level nesting",
			nodes: map[string]*step.ExtractionNode{
				"A": bareNode("A"), "B": bareNode("B"), "C": bareNode("C"),
			},
			rows: []querries.GetLatestExtractionOfDocRow{
				nodeRow("A", ""), nodeRow("B", "A"), nodeRow("C", "B"),
			},
			wantIDs: []string{"A"},
		},
		{
			name: "multiple roots",
			nodes: map[string]*step.ExtractionNode{
				"A": bareNode("A"), "B": bareNode("B"),
			},
			rows: []querries.GetLatestExtractionOfDocRow{
				nodeRow("A", ""), nodeRow("B", ""),
			},
			wantIDs: []string{"A", "B"},
		},
		{
			name:  "missing node in map errors",
			nodes: map[string]*step.ExtractionNode{"A": bareNode("A")},
			rows: []querries.GetLatestExtractionOfDocRow{
				nodeRow("A", ""), nodeRow("B", "A"),
			},
			wantErr: true,
		},
		{
			name:  "missing parent in map errors",
			nodes: map[string]*step.ExtractionNode{"B": bareNode("B")},
			rows: []querries.GetLatestExtractionOfDocRow{
				nodeRow("B", "A"),
			},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			roots, err := wireGraph(tc.nodes, tc.rows)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("wireGraph() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("wireGraph() unexpected error: %v", err)
			}

			gotIDs := make([]string, len(roots))
			for i, r := range roots {
				gotIDs[i] = r.ID
			}
			if diff := cmp.Diff(tc.wantIDs, gotIDs); diff != "" {
				t.Errorf("root IDs mismatch (-want +got):\n%s", diff)
			}

			assertParentLinks(t, tc.nodes, tc.rows)
		})
	}
}

func TestBuildMapThenWireGraph(t *testing.T) {
	rows := []querries.GetLatestExtractionOfDocRow{
		mkRow("#/texts/0", "", "heading", "body", `{"level":1,"text":"Chapter One"}`),
		mkRow("#/texts/1", "#/texts/0", "paragraph", "body", `{"text":"first paragraph"}`),
		mkRow("#/texts/2", "#/texts/0", "paragraph", "body", `{"text":"second paragraph"}`),
		mkRow("#/texts/3", "#/texts/0", "table", "body", ""),
		mkRow("#/texts/4", "", "heading", "body", `{"level":1,"text":"Chapter Two"}`),
	}

	nodes, err := buildMap(rows, testCtx, testLogger)
	if err != nil {
		t.Fatalf("buildMap() unexpected error: %v", err)
	}

	roots, err := wireGraph(nodes, rows)
	if err != nil {
		t.Fatalf("wireGraph() unexpected error: %v", err)
	}

	wantRootIDs := []string{"#/texts/0", "#/texts/4"}
	gotRootIDs := make([]string, len(roots))
	for i, r := range roots {
		gotRootIDs[i] = r.ID
	}
	if diff := cmp.Diff(wantRootIDs, gotRootIDs); diff != "" {
		t.Errorf("root IDs mismatch (-want +got):\n%s", diff)
	}

	want := map[string]*step.ExtractionNode{
		"#/texts/0": {ID: "#/texts/0", Kind: step.KindHeading, Layer: step.LayerBody,
			Heading: &step.HeadingContent{Level: 1, Text: "Chapter One"}},
		"#/texts/1": {ID: "#/texts/1", Kind: step.KindParagraph, Layer: step.LayerBody,
			Paragraph: &step.ParagraphContent{Text: "first paragraph"}},
		"#/texts/2": {ID: "#/texts/2", Kind: step.KindParagraph, Layer: step.LayerBody,
			Paragraph: &step.ParagraphContent{Text: "second paragraph"}},
		"#/texts/3": {ID: "#/texts/3", Kind: step.KindTable, Layer: step.LayerBody},
		"#/texts/4": {ID: "#/texts/4", Kind: step.KindHeading, Layer: step.LayerBody,
			Heading: &step.HeadingContent{Level: 1, Text: "Chapter Two"}},
	}
	if diff := cmp.Diff(want, nodes, cmpopts.IgnoreFields(step.ExtractionNode{}, "Parent", "Children")); diff != "" {
		t.Errorf("node content mismatch (-want +got):\n%s", diff)
	}

	assertParentLinks(t, nodes, rows)
}
