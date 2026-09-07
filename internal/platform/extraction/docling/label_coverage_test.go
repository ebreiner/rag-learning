package docling

import (
	"rag/internal/extract/step"
	"testing"
)

// This file pins the full label surface docling-core defines, as a
// regression net against docling adding/renaming labels underneath us —
// this is exactly how #/tables/0 turned into a dangling ref twice in one
// day. Every label list below was pulled from docling-core's own source
// (docling_core/types/doc/labels.py and .../common/content_layer.py, main
// branch, checked 2026-08-12):
//   https://github.com/docling-project/docling-core
//
// The contract this file enforces: every *currently known* label must
// build without erroring — either into a real Kind (for the ones we
// actually support) or into step.KindUnsupported (for everything else,
// logged and dropped). Plus one "unknown label" case per switch, to check
// that a label docling invents *tomorrow* also degrades gracefully instead
// of taking the whole extraction run down.
//
// If a case here fails, that's the signal: docling changed something,
// go update the corresponding switch in normalize.go, then update this file
// to match the new expected behavior.

// --- texts[] : DocItemLabel ---

func rawTextItemForLabel(label string) rawTextItem {
	item := rawTextItem{
		SelfRef:      "#/texts/0",
		Label:        label,
		Text:         "sample text",
		ContentLayer: string(step.LayerBody),
		Prov:         []rawProv{provOnPage(1)},
	}
	if label == "section_header" || label == "title" {
		level := int64(1)
		item.Level = &level
	}
	return item
}

func TestTextNodeLabelCoverage(t *testing.T) {
	// Every DocItemLabel value, as of docling-core main (2026-08-12).
	cases := []struct {
		label    string
		wantKind step.NodeKind
	}{
		{"text", step.KindParagraph},
		{"section_header", step.KindHeading},
		{"title", step.KindHeading},
		{"list_item", step.KindListItem},
		{"caption", step.KindCaption},
		{"footnote", step.KindFootnote},
		{"page_header", step.KindUnsupported},
		{"page_footer", step.KindUnsupported},
		{"code", step.KindCode},
		{"chart", step.KindUnsupported},
		{"formula", step.KindFormula},
		{"document_index", step.KindUnsupported},
		{"checkbox_selected", step.KindUnsupported},
		{"checkbox_unselected", step.KindUnsupported},
		{"form", step.KindUnsupported},
		{"key_value_region", step.KindUnsupported},
		{"grading_scale", step.KindUnsupported},
		{"handwritten_text", step.KindUnsupported},
		{"empty_value", step.KindUnsupported},
		// "paragraph" is a distinct DocItemLabel from "text" in docling-core's
		// enum, but live corpus logs (844 items, 2026-09-07) showed it is
		// plain body prose in practice, so it folds into the paragraph node.
		{"paragraph", step.KindParagraph},
		{"reference", step.KindUnsupported},
		{"field_region", step.KindUnsupported},
		{"field_heading", step.KindUnsupported},
		{"field_item", step.KindUnsupported},
		{"field_key", step.KindUnsupported},
		{"field_value", step.KindUnsupported},
		{"field_hint", step.KindUnsupported},
		{"marker", step.KindUnsupported},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			node := &step.Node{}
			err := textNode(testCtx, node, rawTextItemForLabel(tc.label), testLogger)
			if err != nil {
				t.Fatalf("label %q: textNode returned unexpected error: %v", tc.label, err)
			}
			if node.Kind != tc.wantKind {
				t.Errorf("label %q: got Kind %q, want %q", tc.label, node.Kind, tc.wantKind)
			}
		})
	}
}

func TestTextNodeUnknownLabelDegradesGracefully(t *testing.T) {
	node := &step.Node{}
	err := textNode(testCtx, node, rawTextItemForLabel("something_docling_invents_tomorrow"), testLogger)
	if err != nil {
		t.Fatalf("unrecognized label should degrade to KindUnsupported, not error: %v", err)
	}
	if node.Kind != step.KindUnsupported {
		t.Errorf("got Kind %q, want %q", node.Kind, step.KindUnsupported)
	}
	if node.ID != "#/texts/0" {
		t.Errorf("node ID must still be set for unsupported labels, got %q", node.ID)
	}
}

// --- groups[] : GroupLabel ---

func rawGroupItemForLabel(label string) rawGroupItem {
	return rawGroupItem{rawDoclingNodeRef{
		SelfRef:      "#/groups/0",
		Label:        label,
		ContentLayer: string(step.LayerBody),
	}}
}

func TestGroupNodeLabelCoverage(t *testing.T) {
	// Every GroupLabel value, as of docling-core main (2026-08-12).
	cases := []struct {
		label    string
		wantKind step.NodeKind
	}{
		{"list", step.KindList},
		{"inline", step.KindGroup},
		{"unspecified", step.KindUnsupported},
		// "ordered_list" is marked deprecated in docling-core; confirm
		// whether it should alias to step.KindList like "list" does.
		{"ordered_list", step.KindUnsupported},
		{"chapter", step.KindUnsupported},
		{"section", step.KindUnsupported},
		{"sheet", step.KindUnsupported},
		{"slide", step.KindUnsupported},
		{"form_area", step.KindUnsupported},
		{"key_value_area", step.KindUnsupported},
		{"comment_section", step.KindUnsupported},
		{"picture_area", step.KindUnsupported},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			node := &step.Node{}
			err := groupNode(testCtx, node, rawGroupItemForLabel(tc.label), testLogger)
			if err != nil {
				t.Fatalf("label %q: groupNode returned unexpected error: %v", tc.label, err)
			}
			if node.Kind != tc.wantKind {
				t.Errorf("label %q: got Kind %q, want %q", tc.label, node.Kind, tc.wantKind)
			}
		})
	}
}

func TestGroupNodeUnknownLabelDegradesGracefully(t *testing.T) {
	node := &step.Node{}
	err := groupNode(testCtx, node, rawGroupItemForLabel("something_docling_invents_tomorrow"), testLogger)
	if err != nil {
		t.Fatalf("unrecognized label should degrade to KindUnsupported, not error: %v", err)
	}
	if node.Kind != step.KindUnsupported {
		t.Errorf("got Kind %q, want %q", node.Kind, step.KindUnsupported)
	}
	if node.ID != "#/groups/0" {
		t.Errorf("node ID must still be set for unsupported labels, got %q", node.ID)
	}
}

// --- tables[] : DocItemLabel (table-relevant subset) ---

func rawTableItemForLabel(label string) rawTableItem {
	return rawTableItem{
		SelfRef:      "#/tables/0",
		Label:        label,
		ContentLayer: string(step.LayerBody),
		Prov:         []rawProv{provWithBBox(1, topLeftBBox(0, 0, 10, 10))},
	}
}

func TestTableNodeLabelCoverage(t *testing.T) {
	heights := pageHeightLookup{1: 100}
	// The only two DocItemLabel values docling actually assigns to tables[]
	// items in practice.
	cases := []struct {
		label    string
		wantKind step.NodeKind
	}{
		{"table", step.KindTable},
		{"document_index", step.KindUnsupported},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			node := &step.Node{}
			err := tableNode(testCtx, node, rawTableItemForLabel(tc.label), heights, testLogger)
			if err != nil {
				t.Fatalf("label %q: tableNode returned unexpected error: %v", tc.label, err)
			}
			if node.Kind != tc.wantKind {
				t.Errorf("label %q: got Kind %q, want %q", tc.label, node.Kind, tc.wantKind)
			}
			// This is exactly the bug that caused "dangling root ref:
			// #/tables/0" twice in one day: a skipped/unsupported table
			// node must still get its real ID, or any root/parent/child ref
			// pointing at it becomes unresolvable in wireGraph.
			if node.ID != "#/tables/0" {
				t.Errorf("label %q: node ID must be set even for unsupported table labels, got %q", tc.label, node.ID)
			}
		})
	}
}

func TestTableNodeUnknownLabelDegradesGracefully(t *testing.T) {
	node := &step.Node{}
	err := tableNode(testCtx, node, rawTableItemForLabel("something_docling_invents_tomorrow"), pageHeightLookup{}, testLogger)
	if err != nil {
		t.Fatalf("unrecognized table label should degrade to KindUnsupported, not error: %v", err)
	}
	if node.Kind != step.KindUnsupported {
		t.Errorf("got Kind %q, want %q", node.Kind, step.KindUnsupported)
	}
	if node.ID != "#/tables/0" {
		t.Errorf("node ID must still be set for unsupported table labels, or refs to this node become dangling -- got %q", node.ID)
	}
}

// --- content_layer : ContentLayer ---

func TestSetContentLayerCoverage(t *testing.T) {
	// Every ContentLayer value, as of docling-core main (2026-08-12).
	// "background"/"invisible"/"notes" have no step.ContentLayer constant
	// yet -- add step.LayerBackground / step.LayerInvisible / step.LayerNotes
	// (mirroring step.LayerBody/step.LayerFurniture) for consistency, then
	// swap these raw casts for the named constants.
	cases := []struct {
		raw  string
		want step.ContentLayer
	}{
		{"body", step.LayerBody},
		{"furniture", step.LayerFurniture},
		{"background", step.ContentLayer("background")},
		{"invisible", step.ContentLayer("invisible")},
		{"notes", step.ContentLayer("notes")},
	}

	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			node := &step.Node{}
			err := setContentLayer(node, tc.raw)
			if err != nil {
				t.Fatalf("content layer %q: setContentLayer returned unexpected error: %v", tc.raw, err)
			}
			if node.Layer != tc.want {
				t.Errorf("content layer %q: got %q, want %q", tc.raw, node.Layer, tc.want)
			}
		})
	}
}
