package docling

import (
	"encoding/base64"
	"rag/internal/extract/step"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type textNodeCase struct {
	name    string
	input   rawTextItem
	want    *step.Node
	wantErr bool
}

type groupNodeCase struct {
	name    string
	input   rawGroupItem
	want    *step.Node
	wantErr bool
}

type pictureNodeCase struct {
	name    string
	input   rawPictureItem
	want    *step.Node
	wantErr bool
}

type tableNodeCase struct {
	name    string
	input   *rawTableItem
	want    *step.Node
	wantErr bool
	heights pageHeightLookup
}

type tableCellCase struct {
	name    string
	input   rawTableCell
	want    step.TableCell
	wantErr bool
}

type buildNodesCase struct {
	name    string
	doc     *rawDoclingDocument
	wantIDs []string
	wantErr bool
}

func TestPictureNodeBuild(t *testing.T) {
	pictureCases := []pictureNodeCase{
		pictureCase("picture becomes picture node", "#/pictures/0", "picture", step.LayerBody, false),
		pictureCase("picture no content layer", "#/pictures/0", "picture", "", true),
		pictureCase("missing self ref errors", "", "picture", step.LayerBody, true),
		pictureCase("unknown label errors", "#/pictures/0", "chart", step.LayerBody, true),
		{
			name: "annotations get copied as strings",
			input: rawPictureItem{
				SelfRef: "#/pictures/0", Label: "picture",
				Prov:         []rawProv{{PageNo: 1}},
				Annotations:  []any{"a dog", "sitting on a chair"},
				ContentLayer: string(step.LayerBody),
			},
			want: &step.Node{
				ID: "#/pictures/0", Kind: step.KindPicture,
				Provenance: []step.Provenance{{Page: 1}},
				Picture:    &step.PictureContent{Annotations: []string{"a dog", "sitting on a chair"}},
				Layer:      step.LayerBody,
			},
		},
		{
			name: "unexpected annotation shape errors",
			input: rawPictureItem{
				SelfRef: "#/pictures/0", Label: "picture",
				Prov:         []rawProv{{PageNo: 1}},
				Annotations:  []any{map[string]any{"text": "not a plain string"}},
				ContentLayer: string(step.LayerBody),
			},
			wantErr: true,
		},
		{
			name: "image gets base64 decoded",
			input: rawPictureItem{
				SelfRef: "#/pictures/0", Label: "picture",
				Prov:         []rawProv{{PageNo: 1}},
				ContentLayer: string(step.LayerBody),
				Image: &rawImage{
					MimeType: "image/png",
					URI:      base64.StdEncoding.EncodeToString([]byte("fake png bytes")),
					Size:     rawSize{Width: 10, Height: 20},
				},
			},
			want: &step.Node{
				ID: "#/pictures/0", Kind: step.KindPicture,
				Provenance: []step.Provenance{{Page: 1}},
				Picture: &step.PictureContent{
					Image: &step.PictureImage{
						MimeType: "image/png",
						Data:     []byte("fake png bytes"),
						Width:    10, Height: 20,
					},
				},
				Layer: step.LayerBody,
			},
		},
		{
			name: "image with a data URI prefix still decodes",
			input: rawPictureItem{
				SelfRef: "#/pictures/0", Label: "picture",
				Prov: []rawProv{{PageNo: 1}},
				Image: &rawImage{
					MimeType: "image/png",
					URI:      "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("fake png bytes")),
					Size:     rawSize{Width: 10, Height: 20},
				},
				ContentLayer: string(step.LayerBody),
			},
			want: &step.Node{
				ID: "#/pictures/0", Kind: step.KindPicture,
				Provenance: []step.Provenance{{Page: 1}},
				Picture: &step.PictureContent{
					Image: &step.PictureImage{
						MimeType: "image/png",
						Data:     []byte("fake png bytes"),
						Width:    10, Height: 20,
					},
				},
				Layer: step.LayerBody,
			},
		},
		{
			name: "invalid base64 image data errors",
			input: rawPictureItem{
				SelfRef: "#/pictures/0", Label: "picture",
				Prov:         []rawProv{{PageNo: 1}},
				Image:        &rawImage{MimeType: "image/png", URI: "not valid base64!!"},
				ContentLayer: string(step.LayerBody),
			},
			wantErr: true,
		},
	}

	runPictureNodeCases(t, pictureCases)
}

func TestGroupNodeBuild(t *testing.T) {

	groupCases := []groupNodeCase{
		groupCase("group becomes group node", "#/groups/1", "list", step.KindList, step.LayerBody, false),
		groupCase("missing self ref errors", "", "list", step.KindGroup, step.LayerBody, true),
		groupCase("group empty content layer", "#/groups/1", "list", step.KindGroup, "", true),
		groupCase("unknown label errors", "#/groups/0", "table", step.KindGroup, step.LayerBody, true),
		groupCase("section is unsupported", "#/groups/0", "section", step.KindUnsupported, step.LayerBody, false),
	}

	runGroupNodeCases(t, groupCases)
}

func TestTextNodeBuild(t *testing.T) {
	validLevel := int64(1)
	invalidLevel := int64(-1)
	upperLimit := int64(6)
	baselineCases := []textNodeCase{
		headingCase("section header becomes heading node", &validLevel, "i am a heading", 1, "i am a heading", step.LayerBody, false),
		headingCase("section node guard header without level", nil, "i am a heading", -1, "", step.LayerBody, true),
		headingCase("section node guard empty content layer", nil, "i am a heading", -1, "", "", true),
		headingCase("section node guard invalid level", &invalidLevel, "i am a heading", -1, "", step.LayerBody, true),
		headingCase("section node guard invalid upper level", &upperLimit, "i am a heading", 6, "", step.LayerBody, true),
		paragraphCase("paragraph becomes paragraph node", "i am representing a full paragraph, hello.", "i am representing a full paragraph, hello.", step.LayerBody, false),
		paragraphCase("paragraph empty content layer", "i am representing a full paragraph, hello.", "i am representing a full paragraph, hello.", "", true),
	}

	runTextNodeCases(t, baselineCases)

	listItemCases := []textNodeCase{
		listItemCase("ordered real marker", boolPtr(true), strPtr("1."), true, "1.", step.LayerBody, false),
		listItemCase("unordered real marker", boolPtr(false), strPtr("•"), false, "•", step.LayerBody, false),
		listItemCase("ordered missing marker", boolPtr(true), nil, true, "-", step.LayerBody, false),
		listItemCase("unordered missing marker", boolPtr(false), nil, false, "-", step.LayerBody, false),
		listItemCase("both unknown", nil, nil, false, "-", step.LayerBody, false),
		listItemCase("list item missing content layer", nil, strPtr("•"), false, "•", "", true),
	}

	runTextNodeCases(t, listItemCases)

	unsupportedCases := []textNodeCase{
		unsupportedTextCase("caption is unsupported", "caption", step.LayerBody),
		unsupportedTextCase("footnote is unsupported", "footnote", step.LayerBody),
		unsupportedTextCase("page_header is unsupported", "page_header", step.LayerFurniture),
		unsupportedTextCase("page_footer is unsupported", "page_footer", step.LayerFurniture),
		unsupportedTextCase("code is unsupported", "code", step.LayerBody),
	}

	runTextNodeCases(t, unsupportedCases)

}

func TestBBoxNormalize(t *testing.T) {
	cases := []struct {
		name    string
		height  float64
		raw     rawBBox
		want    step.BBox
		wantErr bool
	}{
		{"bottomleft flips", 100, rawBBox{Left: 10, Top: 80, Right: 50, Bottom: 20, CoordOrigin: "BOTTOMLEFT"},
			step.BBox{Left: 10, Top: 20, Right: 50, Bottom: 80}, false},
		{"topleft passes through", 100, rawBBox{Left: 10, Top: 20, Right: 50, Bottom: 80, CoordOrigin: "TOPLEFT"},
			step.BBox{Left: 10, Top: 20, Right: 50, Bottom: 80}, false},
		{"unknown origin errors", 100, rawBBox{CoordOrigin: "CENTER"}, step.BBox{}, true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			normalized, err := bboxNormalize(testCase.raw, testCase.height)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}

			if err != nil {
				t.Fatalf("bbox normalize returned unexpected error: %v", err)
			}

			if diff := cmp.Diff(testCase.want, normalized); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}

}

func TestSetContentLayer(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    step.ContentLayer
		wantErr bool
	}{
		{"body maps to layer body", "body", step.LayerBody, false},
		{"furniture maps to layer furniture", "furniture", step.LayerFurniture, false},
		{"empty string errors", "", "", true},
		{"unknown value errors", "unspecified", "", true},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			node := &step.Node{}
			err := setContentLayer(node, testCase.raw)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}
			if err != nil {
				t.Fatalf("setContentLayer returned unexpected error: %v", err)
			}
			if node.Layer != testCase.want {
				t.Errorf("got Layer %q, want %q", node.Layer, testCase.want)
			}
		})
	}
}

func TestTableNodeBuild(t *testing.T) {
	heightLookup := make(pageHeightLookup, 0)
	heightLookup[0] = 100
	heightLookup[1] = 100
	heightLookup[2] = 100
	baselineCases := []tableNodeCase{
		tableCase("valid inputs", "table", "#/tables/1", 1, false, heightLookup, step.LayerBody),
		tableCase("missing self ref", "table", "", 1, true, heightLookup, step.LayerBody),
		tableCase("wrong label", "chart", "#/tables/1", 1, true, heightLookup, step.LayerBody),
		tableCase("no page height", "table", "#/tables/1", 5, true, heightLookup, step.LayerBody),
		tableCase("missing content layer", "table", "#/tables/1", 5, true, heightLookup, ""),
	}

	runTableNodeCases(t, baselineCases)
}

func TestTableCellBuild(t *testing.T) {
	cases := []tableCellCase{
		tableCellCaseValid("valid cell maps all fields"),
		tableCellCaseEmptyText("empty text warns but still builds"),
		tableCellCaseRowSpanMismatch("row span disagreeing with offsets errors"),
		tableCellCaseColSpanMismatch("col span disagreeing with offsets errors"),
		tableCellCaseMissingBBox("missing bbox origin warns, bbox stays nil"),
		tableCellCaseUnknownBBoxOrigin("unexpected bbox origin errors"),
		tableCellCaseHeaderFlags("header and row section flags get copied"),
	}
	runTableCellCases(t, cases)
}

func TestTableProvHandling(t *testing.T) {
	heightLookup := pageHeightLookup{1: 100, 2: 200}
	cases := []tableNodeCase{
		tableProvCase("no prov entries", nil, false, []step.Provenance{}, heightLookup, step.LayerBody),
		tableProvCase("missing content layer", nil, true, []step.Provenance{}, heightLookup, ""),

		tableProvCase("empty coord origin errors",
			[]rawProv{{PageNo: 1}}, true, nil, heightLookup, step.LayerBody),

		tableProvCase("page missing from height lookup errors",
			[]rawProv{provWithBBox(5, topLeftBBox(0, 0, 10, 10))}, true, nil, heightLookup, step.LayerBody),

		tableProvCase("multi page prov uses each page's own height",
			[]rawProv{
				provWithBBox(1, bottomLeftBBox(10, 80, 50, 20)),
				provWithBBox(2, bottomLeftBBox(10, 180, 50, 20)),
			},
			false,
			[]step.Provenance{
				wantProvWithBBox(1, step.BBox{Left: 10, Top: 20, Right: 50, Bottom: 80}),
				wantProvWithBBox(2, step.BBox{Left: 10, Top: 20, Right: 50, Bottom: 180}),
			}, heightLookup, step.LayerBody),

		tableProvCase("charspan present",
			[]rawProv{{PageNo: 1, BBox: topLeftBBox(0, 0, 10, 10), CharSpan: [2]int64{5, 12}}},
			false,
			[]step.Provenance{
				{Page: 1, BBox: &step.BBox{Left: 0, Top: 0, Right: 10, Bottom: 10}, CharSpan: &step.CharSpan{Start: 5, End: 12}},
			}, heightLookup, step.LayerBody),
	}

	runTableNodeCases(t, cases)

}

func boolPtr(b bool) *bool                      { return &b }
func strPtr(s string) *string                   { return &s }
func provOnPage(page int64) rawProv             { return rawProv{PageNo: page} }
func wantProvOnPage(page int64) step.Provenance { return step.Provenance{Page: page} }

func listItemCase(name string, enumerated *bool, marker *string, wantEnumerated bool, wantMarker string, layer step.ContentLayer, wantErr bool) textNodeCase {
	provs := []rawProv{
		provOnPage(1),
	}
	return textNodeCase{
		name: name,
		input: rawTextItem{
			SelfRef: "#/text/1", Label: "list_item",
			Text: "item text", Prov: provs,
			Enumerated: enumerated, Marker: marker,
			ContentLayer: string(layer),
		},
		want: &step.Node{
			ID: "#/text/1", Kind: step.KindListItem,
			Provenance: []step.Provenance{wantProvOnPage(1)},
			ListItem:   &step.ListItemContent{Text: "item text", Enumerated: wantEnumerated, Marker: wantMarker},
			Layer:      layer,
		},
		wantErr: wantErr,
	}
}

func runTextNodeCases(t *testing.T, cases []textNodeCase) {
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			node := &step.Node{}
			err := textNode(node, testCase.input)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}

			if err != nil {
				t.Fatalf("textNode returned error: %v", err)
			}

			if diff := cmp.Diff(testCase.want, node); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func runGroupNodeCases(t *testing.T, cases []groupNodeCase) {
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			node := &step.Node{}
			err := groupNode(node, testCase.input)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}

			if err != nil {
				t.Fatalf("groupNode returned error: %v", err)
			}

			if diff := cmp.Diff(testCase.want, node); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func runTableNodeCases(t *testing.T, cases []tableNodeCase) {
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			node := &step.Node{}
			err := tableNode(node, *testCase.input, testCase.heights)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}
			if err != nil {
				t.Fatal(err)
			}

			if diff := cmp.Diff(testCase.want, node); diff != "" {
				t.Errorf("missmatch (-want +got):\n%s", diff)
			}

		})
	}

}

func TestBuildNodes(t *testing.T) {
	cases := []buildNodesCase{
		{name: "empty document", doc: &rawDoclingDocument{}, wantIDs: []string{}},
		{name: "one of each node type", doc: validRawDoc(),
			wantIDs: []string{"#/texts/0", "#/tables/0", "#/pictures/0", "#/groups/0"}},
		buildNodesCaseFailing("text build failure propagates", func(d *rawDoclingDocument) { d.Texts[0].Label = "unknown" }),
		buildNodesCaseFailing("table build failure propagates", func(d *rawDoclingDocument) { d.Tables[0].SelfRef = "" }),
		buildNodesCaseFailing("picture build failure propagates", func(d *rawDoclingDocument) { d.Pictures[0].Label = "chart" }),
		buildNodesCaseFailing("group build failure propagates", func(d *rawDoclingDocument) { d.Groups[0].Label = "chart" }),
		buildNodesCaseFailing("invalid page key propagates", func(d *rawDoclingDocument) { d.Pages = map[string]rawPage{"abc": {}} }),
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			nodes, err := buildNodes(testCase.doc)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildNodes returned unexpected error: %v", err)
			}

			gotIDs := make([]string, 0, len(nodes))
			for id := range nodes {
				gotIDs = append(gotIDs, id)
			}
			sort.Strings(gotIDs)
			sort.Strings(testCase.wantIDs)
			if diff := cmp.Diff(testCase.wantIDs, gotIDs); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func headingCase(name string, level *int64, text string, wantLevel int64, wantText string, layer step.ContentLayer, wantErr bool) textNodeCase {
	provs := []rawProv{provOnPage(1)}
	return textNodeCase{
		name: name,
		input: rawTextItem{
			SelfRef: "#/text/1", Label: "section_header",
			Level: level, Text: text, Prov: provs,
			ContentLayer: string(layer),
		},
		want: &step.Node{
			ID: "#/text/1", Kind: step.KindHeading,
			Provenance: []step.Provenance{wantProvOnPage(1)},
			Heading:    &step.HeadingContent{Level: wantLevel, Text: wantText},
			Layer:      layer,
		},
		wantErr: wantErr,
	}
}

func paragraphCase(name string, text string, wantText string, layer step.ContentLayer, wantErr bool) textNodeCase {
	provs := []rawProv{provOnPage(1)}
	return textNodeCase{
		name: name,
		input: rawTextItem{
			SelfRef: "#/text/1", Label: "text",
			Text: text, Prov: provs,
			ContentLayer: string(layer),
		},
		want: &step.Node{
			ID: "#/text/1", Kind: step.KindParagraph,
			Provenance: []step.Provenance{wantProvOnPage(1)},
			Paragraph:  &step.ParagraphContent{Text: wantText},
			Layer:      layer,
		},
		wantErr: wantErr,
	}
}

func groupCase(name, selfRef, label string, wantsKind step.NodeKind, layer step.ContentLayer, wantErr bool) groupNodeCase {
	return groupNodeCase{
		name: name,
		input: rawGroupItem{
			rawDoclingNodeRef{
				SelfRef:      selfRef,
				Label:        label,
				ContentLayer: string(layer),
			},
		},
		wantErr: wantErr,
		want: &step.Node{
			ID:    selfRef,
			Kind:  wantsKind,
			Layer: layer,
		},
	}
}

func runPictureNodeCases(t *testing.T, cases []pictureNodeCase) {
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			node := &step.Node{}
			err := pictureNode(node, testCase.input)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}

			if err != nil {
				t.Fatalf("pictureNode returned error: %v", err)
			}

			if diff := cmp.Diff(testCase.want, node); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func pictureCase(name, selfRef, label string, contentLayer step.ContentLayer, wantErr bool) pictureNodeCase {
	return pictureNodeCase{
		name: name,
		input: rawPictureItem{
			SelfRef: selfRef, Label: label,
			Prov:         []rawProv{provOnPage(1)},
			ContentLayer: string(contentLayer),
		},
		wantErr: wantErr,
		want: &step.Node{
			ID:         selfRef,
			Kind:       step.KindPicture,
			Provenance: []step.Provenance{wantProvOnPage(1)},
			Picture:    &step.PictureContent{},
			Layer:      contentLayer,
		},
	}
}

func tableCase(name, label, selfRef string, pageNo int, wantErr bool, heightLookup pageHeightLookup, layer step.ContentLayer) tableNodeCase {
	bbox := rawBBox{
		CoordOrigin: "TOPLEFT",
		Top:         0,
		Left:        0,
		Right:       10,
		Bottom:      10,
	}
	provRaw := rawProv{PageNo: int64(pageNo), BBox: bbox}
	prov := wantProvOnPage(1)
	prov.BBox = &step.BBox{Left: 0, Top: 0, Bottom: 10, Right: 10}
	cells := make([]step.TableCell, 0, 0)
	tableContent := &step.TableContent{Cells: cells}
	tableCase := tableNodeCase{
		name: name,
		input: &rawTableItem{
			Label:        label,
			Prov:         []rawProv{provRaw},
			SelfRef:      selfRef,
			ContentLayer: string(layer),
		},
		want: &step.Node{
			ID:         selfRef,
			Kind:       step.KindTable,
			Provenance: []step.Provenance{prov},
			Layer:      layer,
			Table:      tableContent,
		},
		wantErr: wantErr,
		heights: heightLookup,
	}

	return tableCase
}

func bottomLeftBBox(l, t, r, b float64) rawBBox {
	return rawBBox{Left: l, Top: t, Right: r, Bottom: b, CoordOrigin: "BOTTOMLEFT"}
}
func topLeftBBox(l, t, r, b float64) rawBBox {
	return rawBBox{Left: l, Top: t, Right: r, Bottom: b, CoordOrigin: "TOPLEFT"}
}
func provWithBBox(page int64, bbox rawBBox) rawProv { return rawProv{PageNo: page, BBox: bbox} }
func wantProvWithBBox(page int64, bbox step.BBox) step.Provenance {
	return step.Provenance{Page: page, BBox: &bbox}
}

func tableProvCase(name string, provs []rawProv, wantErr bool, want []step.Provenance, heightLookup pageHeightLookup, layer step.ContentLayer) tableNodeCase {
	cells := make([]step.TableCell, 0, 0)
	tableContent := &step.TableContent{Cells: cells}
	return tableNodeCase{
		name:    name,
		wantErr: wantErr,
		input:   &rawTableItem{SelfRef: "#/tables/1", Label: "table", Prov: provs, ContentLayer: string(layer)},
		want:    &step.Node{ID: "#/tables/1", Kind: step.KindTable, Provenance: want, Layer: layer, Table: tableContent},
		heights: heightLookup,
	}
}

func validRawCell() rawTableCell {
	return rawTableCell{
		Text:              "cell text",
		BBox:              topLeftBBox(0, 0, 10, 10),
		RowSpan:           1,
		ColSpan:           1,
		StartRowOffsetIdx: 0,
		EndRowOffsetIdx:   1,
		StartColOffsetIdx: 0,
		EndColOffsetIdx:   1,
		Fillable:          true,
	}
}

func validWantCell() step.TableCell {
	return step.TableCell{
		Text:     "cell text",
		BBox:     &step.BBox{Left: 0, Top: 0, Right: 10, Bottom: 10},
		RowStart: 0, RowEnd: 1,
		ColStart: 0, ColEnd: 1,
		Fillable: true,
	}
}

func tableCellCaseValid(name string) tableCellCase {
	return tableCellCase{name: name, input: validRawCell(), want: validWantCell()}
}

func tableCellCaseEmptyText(name string) tableCellCase {
	raw := validRawCell()
	raw.Text = ""
	want := validWantCell()
	want.Text = ""
	return tableCellCase{name: name, input: raw, want: want}
}

func tableCellCaseRowSpanMismatch(name string) tableCellCase {
	raw := validRawCell()
	raw.RowSpan = 2
	return tableCellCase{name: name, input: raw, wantErr: true}
}

func tableCellCaseColSpanMismatch(name string) tableCellCase {
	raw := validRawCell()
	raw.ColSpan = 2
	return tableCellCase{name: name, input: raw, wantErr: true}
}

func tableCellCaseMissingBBox(name string) tableCellCase {
	raw := validRawCell()
	raw.BBox = rawBBox{}
	want := validWantCell()
	want.BBox = nil
	return tableCellCase{name: name, input: raw, want: want}
}

func tableCellCaseUnknownBBoxOrigin(name string) tableCellCase {
	raw := validRawCell()
	raw.BBox = bottomLeftBBox(0, 0, 10, 10)
	return tableCellCase{name: name, input: raw, wantErr: true}
}

func tableCellCaseHeaderFlags(name string) tableCellCase {
	raw := validRawCell()
	raw.ColumnHeader = true
	raw.RowHeader = true
	raw.RowSection = true
	want := validWantCell()
	want.IsColumnHeader = true
	want.IsRowHeader = true
	want.RowSection = true
	return tableCellCase{name: name, input: raw, want: want}
}

func runTableCellCases(t *testing.T, cases []tableCellCase) {
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := tableCell(testCase.input)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				} else {
					return
				}
			}
			if err != nil {
				t.Fatalf("tableCell returned error: %v", err)
			}
			if diff := cmp.Diff(testCase.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func validRawDoc() *rawDoclingDocument {
	return &rawDoclingDocument{
		Pages: map[string]rawPage{"1": {Size: rawSize{Width: 600, Height: 100}}},
		Texts: []rawTextItem{
			{SelfRef: "#/texts/0", Label: "text", Text: "hello", ContentLayer: "body", Prov: []rawProv{{PageNo: 1}}},
		},
		Tables: []rawTableItem{
			{SelfRef: "#/tables/0", Label: "table", ContentLayer: "body", Prov: []rawProv{provWithBBox(1, topLeftBBox(0, 0, 10, 10))}},
		},
		Pictures: []rawPictureItem{
			{SelfRef: "#/pictures/0", Label: "picture", ContentLayer: "body", Prov: []rawProv{{PageNo: 1}}},
		},
		Groups: []rawGroupItem{
			{rawDoclingNodeRef{SelfRef: "#/groups/0", Label: "list", ContentLayer: "body"}},
		},
	}
}

func buildNodesCaseFailing(name string, mutate func(*rawDoclingDocument)) buildNodesCase {
	doc := validRawDoc()
	mutate(doc)
	return buildNodesCase{name: name, doc: doc, wantErr: true}
}

func unsupportedTextCase(name, label string, layer step.ContentLayer) textNodeCase {
	provs := []rawProv{provOnPage(1)}
	return textNodeCase{
		name: name,
		input: rawTextItem{
			SelfRef: "#/text/1", Label: label,
			Text: "irrelevant for unsupported labels", Prov: provs,
			ContentLayer: string(layer),
		},
		want: &step.Node{
			ID:         "#/text/1",
			Kind:       step.KindUnsupported,
			Provenance: []step.Provenance{wantProvOnPage(1)},
			Layer:      layer,
		},
	}
}
