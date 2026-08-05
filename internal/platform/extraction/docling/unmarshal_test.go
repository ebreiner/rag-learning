package docling

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUnmarshallNodeKindCounts(t *testing.T) {
	// jq + count of nodes in extraction response
	tests := []struct {
		name         string
		fixture      string
		wantTexts    int
		wantGroups   int
		wantTables   int
		wantPictures int
		wantPages    int
		wantMimeType string
	}{
		{"baseline", "baseline_extracted.json", 15, 0, 0, 0, 2, "application/pdf"},
		{"empty", "empty_extracted.json", 1, 0, 0, 0, 1, "application/pdf"},
		{"footnote", "footnote_extracted.json", 9, 0, 0, 0, 2, "application/pdf"},
		{"furniture", "furniture_extracted.json", 15, 0, 0, 0, 3, "application/pdf"},
		{"lists", "lists_extracted.json", 14, 2, 0, 0, 1, "application/pdf"},
		{"picture", "picture_extracted.json", 3, 0, 0, 1, 1, "application/pdf"},
		{"table", "table_extracted.json", 3, 0, 1, 0, 1, "application/pdf"},
		{"footnote_no_heading", "footnote_no_heading_extracted.json", 8, 0, 0, 0, 2, "application/pdf"},
		{"footnote_minimal", "footnote_minimal_extracted.json", 2, 0, 0, 0, 1, "application/pdf"},
		{"footnote_css_generated", "footnote_css_generated_extracted.json", 4, 1, 0, 0, 1, "application/pdf"},
		{"nested_lists", "nested_lists_extracted.json", 14, 2, 0, 0, 1, "application/pdf"},
		{"kitchen_sink", "kitchen_sink_extracted.json", 10, 0, 2, 0, 2, "application/pdf"},
		{"inline_image", "inline_image_extracted.json", 3, 0, 0, 0, 1, "application/pdf"},
	}

	for _, fixture := range tests {
		t.Run(fixture.name, func(t *testing.T) {
			doc := loadFixture(t, fixture.fixture)
			if got := len(doc.Texts); got != fixture.wantTexts {
				t.Errorf("Texts = %d, want %d", got, fixture.wantTexts)
			}
			if got := len(doc.Groups); got != fixture.wantGroups {
				t.Errorf("Groups = %d, want %d", got, fixture.wantGroups)
			}
			if got := len(doc.Tables); got != fixture.wantTables {
				t.Errorf("Tables = %d, want %d", got, fixture.wantTables)
			}
			if got := len(doc.Pictures); got != fixture.wantPictures {
				t.Errorf("Pictures = %d, want %d", got, fixture.wantPictures)
			}
			if got := len(doc.Pages); got != fixture.wantPages {
				t.Errorf("Pages = %d, want %d", got, fixture.wantPages)
			}

			if got := doc.Origin.MimeType; got != fixture.wantMimeType {
				t.Errorf("mime_type = %s, want %s", got, fixture.wantMimeType)
			}

		})
	}
}

func loadFixture(t *testing.T, name string) rawDoclingDocument {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	var resp rawConvertResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("unmarshaling fixture %s: %v", name, err)
	}

	return *resp.Document.JSONContent
}
