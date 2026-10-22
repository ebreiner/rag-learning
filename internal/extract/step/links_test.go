package step

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestValidateLinks(t *testing.T) {
	titled := map[string]any{KeyTitle: "Process list"}
	manyLinks := make([]Link, maxLinks+1)
	for i := range manyLinks {
		manyLinks[i] = Link{To: "a.md"}
	}

	tests := []struct {
		name     string
		doc      SourceDoc
		want     []Link
		wantErr  bool
		errMatch string
	}{
		{
			name: "no links key gives nil and needs no title",
			doc:  SourceDoc{RelPath: "a.md", Metadata: map[string]any{}},
			want: nil,
		},
		{
			name: "an empty links list is kept as empty, not nil",
			doc:  SourceDoc{RelPath: "a.md", Metadata: titled, Links: []Link{}},
			want: []Link{},
		},
		{
			name: "valid links keep their order",
			doc: SourceDoc{RelPath: "list.md", Metadata: titled, Links: []Link{
				{To: "b.md", Text: "B", Section: "Admin"},
				{To: "dir/c.md"},
			}},
			want: []Link{{To: "b.md", Text: "B", Section: "Admin"}, {To: "dir/c.md"}},
		},
		{
			name: "a self link is dropped",
			doc:  SourceDoc{RelPath: "list.md", Metadata: titled, Links: []Link{{To: "list.md"}, {To: "b.md"}}},
			want: []Link{{To: "b.md"}},
		},
		{
			name: "a duplicate in the same section is dropped, the first text wins",
			doc: SourceDoc{RelPath: "list.md", Metadata: titled, Links: []Link{
				{To: "b.md", Text: "first", Section: "S"},
				{To: "b.md", Text: "second", Section: "S"},
			}},
			want: []Link{{To: "b.md", Text: "first", Section: "S"}},
		},
		{
			name: "the same target in two sections gives two links",
			doc: SourceDoc{RelPath: "list.md", Metadata: titled, Links: []Link{
				{To: "b.md", Section: "Current"},
				{To: "b.md", Section: "Review needed"},
			}},
			want: []Link{{To: "b.md", Section: "Current"}, {To: "b.md", Section: "Review needed"}},
		},
		{
			name:     "links without doc.title are rejected",
			doc:      SourceDoc{RelPath: "a.md", Metadata: map[string]any{}, Links: []Link{{To: "b.md"}}},
			wantErr:  true,
			errMatch: KeyTitle,
		},
		{
			name:     "a doc.title that is not a string is rejected",
			doc:      SourceDoc{RelPath: "a.md", Metadata: map[string]any{KeyTitle: 3.0}, Links: []Link{{To: "b.md"}}},
			wantErr:  true,
			errMatch: KeyTitle,
		},
		{
			name:     "an empty links list also needs doc.title",
			doc:      SourceDoc{RelPath: "a.md", Metadata: map[string]any{KeyTitle: " "}, Links: []Link{}},
			wantErr:  true,
			errMatch: KeyTitle,
		},
		{name: "empty to", doc: SourceDoc{Metadata: titled, Links: []Link{{To: ""}}}, wantErr: true, errMatch: "links[0].to"},
		{name: "absolute to", doc: SourceDoc{Metadata: titled, Links: []Link{{To: "/etc/a.md"}}}, wantErr: true, errMatch: "absolute"},
		{name: "unclean to", doc: SourceDoc{Metadata: titled, Links: []Link{{To: "./a.md"}}}, wantErr: true, errMatch: "clean"},
		{name: "to outside the source", doc: SourceDoc{Metadata: titled, Links: []Link{{To: "../a.md"}}}, wantErr: true, errMatch: "outside"},
		{
			name:     "a text longer than 300 characters",
			doc:      SourceDoc{Metadata: titled, Links: []Link{{To: "a.md", Text: strings.Repeat("ä", maxLinkTextLen+1)}}},
			wantErr:  true,
			errMatch: "links[0].text",
		},
		{
			name:     "a section longer than 300 characters",
			doc:      SourceDoc{Metadata: titled, Links: []Link{{To: "a.md", Section: strings.Repeat("s", maxLinkTextLen+1)}}},
			wantErr:  true,
			errMatch: "links[0].section",
		},
		{
			name:     "more than 1000 links",
			doc:      SourceDoc{Metadata: titled, Links: manyLinks},
			wantErr:  true,
			errMatch: "max. 1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateLinks(tt.doc)
			if tt.wantErr != (err != nil) {
				t.Fatalf("validateLinks() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				if !errors.Is(err, ErrInvalidMetadata) {
					t.Errorf("validateLinks() error = %v, want ErrInvalidMetadata", err)
				}
				if !strings.Contains(err.Error(), tt.errMatch) {
					t.Errorf("validateLinks() error = %q, want it to contain %q", err, tt.errMatch)
				}
				return
			}
			if (tt.want == nil) != (got == nil) {
				t.Fatalf("validateLinks() = %#v, want %#v (nil and empty differ)", got, tt.want)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("validateLinks() (-want +got):\n%s", diff)
			}
		})
	}
}
