package docling

import (
	"github.com/google/go-cmp/cmp"
	"rag/internal/extract/step"
	"testing"
)

func TestWireNode(t *testing.T) {
	other := &step.Node{ID: "#/texts/9"}
	childA := &step.Node{ID: "#/texts/a"}
	childB := &step.Node{ID: "#/texts/b"}
	flatNodes := map[string]*step.Node{
		"#/texts/9": other,
		"#/texts/a": childA,
		"#/texts/b": childB,
	}

	cases := []struct {
		name       string
		parent     *rawRef
		children   []rawRef
		wantParent *step.Node
		wantErr    bool
	}{
		{"nil parent leaves Parent nil", nil, nil, other, false},
		{"empty parent ref errors", &rawRef{Ref: ""}, nil, nil, true},
		{"body ref makes node a root", &rawRef{Ref: "#/body"}, nil, nil, false},
		{"furniture ref makes node a root", &rawRef{Ref: "#/furniture"}, nil, nil, false},
		{"real parent ref resolves", &rawRef{Ref: "#/texts/9"}, nil, other, false},
		{"unresolvable parent ref errors", &rawRef{Ref: "#/missing"}, nil, nil, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			node := &step.Node{ID: "#/texts/0", Parent: other} // pre-set so we can tell cleared from untouched
			err := wireNode(node, testCase.parent, testCase.children, flatNodes)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}
			if err != nil {
				t.Fatalf("wireNode returned unexpected error: %v", err)
			}
			if diff := cmp.Diff(node.Parent, testCase.wantParent); diff != "" {
				t.Errorf("(-want +got): %s", diff)
			}
		})
	}

	t.Run("children resolve in order", func(t *testing.T) {
		node := &step.Node{ID: "#/texts/0"}
		err := wireNode(node, nil, []rawRef{{Ref: "#/texts/a"}, {Ref: "#/texts/b"}}, flatNodes)
		if err != nil {
			t.Fatalf("wireNode returned unexpected error: %v", err)
		}
		if len(node.Children) != 2 || node.Children[0] != childA || node.Children[1] != childB {
			t.Errorf("got Children %v, want [childA, childB] in order", node.Children)
		}
	})

	t.Run("dangling child ref errors", func(t *testing.T) {
		node := &step.Node{ID: "#/texts/0"}
		err := wireNode(node, nil, []rawRef{{Ref: "#/missing"}}, flatNodes)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("dangling child ref errors", func(t *testing.T) {
		node := &step.Node{ID: "#/texts/0"}
		err := wireNode(node, nil, []rawRef{{Ref: "#/missing"}}, flatNodes)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("no children produces non-nil empty slice", func(t *testing.T) {
		node := &step.Node{ID: "#/texts/0"}
		if err := wireNode(node, nil, nil, flatNodes); err != nil {
			t.Fatalf("wireNode returned unexpected error: %v", err)
		}
		if node.Children == nil || len(node.Children) != 0 {
			t.Errorf("got Children %v, want non-nil empty slice", node.Children)
		}
	})
}

func TestWireGraph(t *testing.T) {
	heading := &step.Node{ID: "#/texts/0", Kind: step.KindHeading}
	paragraph := &step.Node{ID: "#/texts/1", Kind: step.KindParagraph}
	table := &step.Node{ID: "#/tables/0", Kind: step.KindTable}
	picture := &step.Node{ID: "#/pictures/0", Kind: step.KindPicture}
	group := &step.Node{ID: "#/groups/0", Kind: step.KindList}
	flatNodes := map[string]*step.Node{
		heading.ID: heading, paragraph.ID: paragraph,
		table.ID: table, picture.ID: picture, group.ID: group,
	}

	rawDoc := &rawDoclingDocument{
		Body: rawDoclingNodeRef{
			SelfRef: "#/body",
			Children: []rawRef{
				{Ref: "#/texts/0"}, {Ref: "#/tables/0"}, {Ref: "#/pictures/0"}, {Ref: "#/groups/0"},
			},
		},
		Furniture: rawDoclingNodeRef{SelfRef: "#/furniture"},
		Texts: []rawTextItem{
			{SelfRef: "#/texts/0", Children: []rawRef{{Ref: "#/texts/1"}}},
			{SelfRef: "#/texts/1", Parent: &rawRef{Ref: "#/texts/0"}},
		},
		Tables:   []rawTableItem{{SelfRef: "#/tables/0"}},
		Pictures: []rawPictureItem{{SelfRef: "#/pictures/0"}},
		Groups:   []rawGroupItem{{rawDoclingNodeRef{SelfRef: "#/groups/0"}}},
	}

	got, err := wireGraph(flatNodes, rawDoc)
	if err != nil {
		t.Fatalf("wireGraph returned unexpected error: %v", err)
	}

	wantRootIDs := []string{"#/texts/0", "#/tables/0", "#/pictures/0", "#/groups/0"}
	gotRootIDs := make([]string, len(got.RootNodes))
	for i, n := range got.RootNodes {
		gotRootIDs[i] = n.ID
	}
	if diff := cmp.Diff(wantRootIDs, gotRootIDs); diff != "" {
		t.Errorf("root order mismatch (-want +got):\n%s", diff)
	}

	if paragraph.Parent != heading {
		t.Errorf("paragraph.Parent = %v, want heading", paragraph.Parent)
	}
	if len(heading.Children) != 1 || heading.Children[0] != paragraph {
		t.Errorf("heading.Children = %v, want [paragraph]", heading.Children)
	}
	if table.Parent != nil || picture.Parent != nil || group.Parent != nil {
		t.Error("root nodes should have nil Parent")
	}
}

// Docling links captions/footnotes to their host through the host's own
// captions/footnotes ref lists, while the caption item itself sits in
// body.children with parent "#/body" (confirmed live 2026-09-08 on the manual
// corpus: 785 captions, all picture-hosted, zero of them in children[]).
// wireGraph must re-parent them under the host so walk()'s children-based
// consumption path can see them, and drop them from the roots.
func TestWireGraphReparentsCaptionsAndFootnotesUnderHost(t *testing.T) {
	picture := &step.Node{ID: "#/pictures/0", Kind: step.KindPicture}
	picCaption := &step.Node{ID: "#/texts/0", Kind: step.KindCaption}
	table := &step.Node{ID: "#/tables/0", Kind: step.KindTable}
	tableCaption := &step.Node{ID: "#/texts/1", Kind: step.KindCaption}
	tableFootnote := &step.Node{ID: "#/texts/2", Kind: step.KindFootnote}
	after := &step.Node{ID: "#/texts/3", Kind: step.KindParagraph}
	flatNodes := map[string]*step.Node{
		picture.ID: picture, picCaption.ID: picCaption,
		table.ID: table, tableCaption.ID: tableCaption, tableFootnote.ID: tableFootnote,
		after.ID: after,
	}

	body := &rawRef{Ref: "#/body"}
	rawDoc := &rawDoclingDocument{
		Body: rawDoclingNodeRef{
			SelfRef: "#/body",
			Children: []rawRef{
				{Ref: "#/pictures/0"}, {Ref: "#/texts/0"},
				{Ref: "#/tables/0"}, {Ref: "#/texts/1"}, {Ref: "#/texts/2"},
				{Ref: "#/texts/3"},
			},
		},
		Furniture: rawDoclingNodeRef{SelfRef: "#/furniture"},
		Texts: []rawTextItem{
			{SelfRef: "#/texts/0", Parent: body},
			{SelfRef: "#/texts/1", Parent: body},
			{SelfRef: "#/texts/2", Parent: body},
			{SelfRef: "#/texts/3", Parent: body},
		},
		Tables: []rawTableItem{{
			SelfRef: "#/tables/0", Parent: body,
			Captions:  []rawRef{{Ref: "#/texts/1"}},
			Footnotes: []rawRef{{Ref: "#/texts/2"}},
		}},
		Pictures: []rawPictureItem{{
			SelfRef: "#/pictures/0", Parent: body,
			Captions: []rawRef{{Ref: "#/texts/0"}},
		}},
	}

	got, err := wireGraph(flatNodes, rawDoc)
	if err != nil {
		t.Fatalf("wireGraph returned unexpected error: %v", err)
	}

	wantRootIDs := []string{"#/pictures/0", "#/tables/0", "#/texts/3"}
	gotRootIDs := make([]string, len(got.RootNodes))
	for i, n := range got.RootNodes {
		gotRootIDs[i] = n.ID
	}
	if diff := cmp.Diff(wantRootIDs, gotRootIDs); diff != "" {
		t.Errorf("captions/footnotes must leave the roots (-want +got):\n%s", diff)
	}

	if picCaption.Parent != picture {
		t.Errorf("picture caption Parent = %v, want the picture", picCaption.Parent)
	}
	if len(picture.Children) != 1 || picture.Children[0] != picCaption {
		t.Errorf("picture.Children = %v, want [caption]", picture.Children)
	}

	if tableCaption.Parent != table || tableFootnote.Parent != table {
		t.Errorf("table caption/footnote Parent = %v / %v, want the table", tableCaption.Parent, tableFootnote.Parent)
	}
	if len(table.Children) != 2 || table.Children[0] != tableCaption || table.Children[1] != tableFootnote {
		t.Errorf("table.Children = %v, want [caption footnote]", table.Children)
	}

	if after.Parent != nil {
		t.Errorf("unrelated body paragraph must stay a root, got Parent = %v", after.Parent)
	}
}

// If a docling build ever lists the caption in the host's children[] AND in
// captions[] (the shape an older comment in unmarshal.go claimed to have seen),
// the caption must not end up in Children twice.
func TestWireGraphDoesNotDuplicateCaptionAlreadyWiredAsChild(t *testing.T) {
	picture := &step.Node{ID: "#/pictures/0", Kind: step.KindPicture}
	caption := &step.Node{ID: "#/texts/0", Kind: step.KindCaption}
	flatNodes := map[string]*step.Node{picture.ID: picture, caption.ID: caption}

	rawDoc := &rawDoclingDocument{
		Body: rawDoclingNodeRef{
			SelfRef:  "#/body",
			Children: []rawRef{{Ref: "#/pictures/0"}},
		},
		Furniture: rawDoclingNodeRef{SelfRef: "#/furniture"},
		Texts: []rawTextItem{
			{SelfRef: "#/texts/0", Parent: &rawRef{Ref: "#/pictures/0"}},
		},
		Pictures: []rawPictureItem{{
			SelfRef:  "#/pictures/0",
			Parent:   &rawRef{Ref: "#/body"},
			Children: []rawRef{{Ref: "#/texts/0"}},
			Captions: []rawRef{{Ref: "#/texts/0"}},
		}},
	}

	got, err := wireGraph(flatNodes, rawDoc)
	if err != nil {
		t.Fatalf("wireGraph returned unexpected error: %v", err)
	}

	if len(got.RootNodes) != 1 || got.RootNodes[0] != picture {
		t.Errorf("roots = %v, want [picture]", got.RootNodes)
	}
	if len(picture.Children) != 1 || picture.Children[0] != caption {
		t.Errorf("picture.Children = %v, want exactly [caption] once", picture.Children)
	}
	if caption.Parent != picture {
		t.Errorf("caption.Parent = %v, want the picture", caption.Parent)
	}
}

func TestWireGraphDanglingCaptionRefIsAnError(t *testing.T) {
	picture := &step.Node{ID: "#/pictures/0", Kind: step.KindPicture}
	flatNodes := map[string]*step.Node{picture.ID: picture}

	rawDoc := &rawDoclingDocument{
		Body:      rawDoclingNodeRef{SelfRef: "#/body", Children: []rawRef{{Ref: "#/pictures/0"}}},
		Furniture: rawDoclingNodeRef{SelfRef: "#/furniture"},
		Pictures: []rawPictureItem{{
			SelfRef:  "#/pictures/0",
			Parent:   &rawRef{Ref: "#/body"},
			Captions: []rawRef{{Ref: "#/texts/99"}},
		}},
	}

	if _, err := wireGraph(flatNodes, rawDoc); err == nil {
		t.Fatalf("expected an error for a caption ref pointing at a missing node, got nil")
	}
}
