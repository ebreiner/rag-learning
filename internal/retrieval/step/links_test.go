package step

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type fakeLinkLoader struct {
	data      LinkData
	err       error
	gotDocIDs []int64
	gotFilter Filter
	calls     int
}

func (f *fakeLinkLoader) LoadLinks(ctx context.Context, docIDs []int64, filter Filter) (LinkData, error) {
	f.calls++
	f.gotDocIDs = docIDs
	f.gotFilter = filter
	return f.data, f.err
}

func ptr(v int64) *int64 { return &v }

// hubLinks gives n links in each of the sections, with chunk IDs from 1000 on.
func hubLinks(sections []string, n int) []LinkedDoc {
	var out []LinkedDoc
	id := int64(1000)
	for _, s := range sections {
		for i := range n {
			out = append(out, LinkedDoc{Section: s, Name: fmt.Sprintf("%s-%d.md", s, i), Metadata: map[string]any{}, ChunkID: id})
			id++
		}
	}
	return out
}

func TestAttachLinks(t *testing.T) {
	ctx := context.Background()
	titled := map[string]any{"doc.title": "Hub"}

	t.Run("no chunks: empty result, no loader call", func(t *testing.T) {
		loader := &fakeLinkLoader{}
		got, err := AttachLinks(ctx, nil, Filter{}, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		if loader.calls != 0 || len(got.Chunks) != 0 || len(got.Documents) != 0 {
			t.Errorf("got %+v with %d loader calls, want an empty result and 0 calls", got, loader.calls)
		}
		raw, _ := json.Marshal(got)
		if string(raw) != `{"chunks":[],"documents":{}}` {
			t.Errorf("JSON = %s, want empty list and empty map", raw)
		}
	})

	t.Run("each document appears one time, the loader gets each ID one time and the filter", func(t *testing.T) {
		loader := &fakeLinkLoader{data: LinkData{}}
		chunks := []RetrievedChunk{
			{ID: 1, DocID: 7, DocTitle: "a.md", SourceName: "wiki", Metadata: map[string]any{"doc.title": "A"}},
			{ID: 2, DocID: 8, DocTitle: "b.md", SourceName: "manual", Metadata: map[string]any{}},
			{ID: 3, DocID: 7, DocTitle: "a.md", SourceName: "wiki", Metadata: map[string]any{"doc.title": "A"}},
		}
		filter := Filter{Sources: []string{"wiki"}}
		got, err := AttachLinks(ctx, chunks, filter, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		if diff := cmp.Diff([]int64{7, 8}, loader.gotDocIDs); diff != "" {
			t.Errorf("loader doc IDs (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(filter, loader.gotFilter); diff != "" {
			t.Errorf("loader filter (-want +got):\n%s", diff)
		}
		want := map[string]SearchDocument{
			"7": {Title: "A", Source: "wiki", Metadata: map[string]any{"doc.title": "A"}, LinksOut: []LinkGroup{}, LinksIn: []LinkEntry{}},
			"8": {Title: "b.md", Source: "manual", Metadata: map[string]any{}, LinksOut: []LinkGroup{}, LinksIn: []LinkEntry{}},
		}
		if diff := cmp.Diff(want, got.Documents); diff != "" {
			t.Errorf("documents (-want +got):\n%s", diff)
		}
		if len(got.Chunks) != 3 {
			t.Errorf("chunks = %d, want 3 unchanged", len(got.Chunks))
		}
	})

	t.Run("a small document has all groups open, in the order of the first link", func(t *testing.T) {
		loader := &fakeLinkLoader{data: LinkData{
			Out: map[int64][]LinkedDoc{7: {
				{Section: "B", Name: "x.md", Metadata: map[string]any{"doc.title": "X"}, ChunkID: 100},
				{Section: "", Name: "y.md", Metadata: map[string]any{}, ChunkID: 200, Text: "why"},
				{Section: "B", Name: "z.md", Metadata: map[string]any{}, ChunkID: 300},
			}},
			Sections: map[int64][]SectionStart{7: {{"Hub", 10}, {"Hub > B", 11}, {"Hub > B", 12}}},
		}}
		chunks := []RetrievedChunk{{ID: 10, DocID: 7, Breadcrumb: "Hub", Metadata: titled}}
		got, err := AttachLinks(ctx, chunks, Filter{}, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		want := []LinkGroup{
			{Section: "B", Total: 2, ChunkID: ptr(11), Links: []LinkEntry{{Title: "X", ChunkID: 100}, {Title: "z.md", ChunkID: 300}}},
			{Section: "", Total: 1, ChunkID: ptr(10), Links: []LinkEntry{{Title: "y.md", Text: "why", ChunkID: 200}}},
		}
		doc := got.Documents["7"]
		if diff := cmp.Diff(want, doc.LinksOut); diff != "" {
			t.Errorf("links_out (-want +got):\n%s", diff)
		}
		if doc.LinksOutTotal != 3 {
			t.Errorf("links_out_total = %d, want 3", doc.LinksOutTotal)
		}
	})

	t.Run("a hub opens only the group of the hit section, by exact match", func(t *testing.T) {
		out := hubLinks([]string{"Docs", "Docs > Review needed", "Admin"}, 4)
		loader := &fakeLinkLoader{data: LinkData{
			Out: map[int64][]LinkedDoc{7: out},
			// the title prefix is in some breadcrumbs only, as docling makes it
			Sections: map[int64][]SectionStart{7: {{"Docs", 20}, {"Hub > Docs > Review needed", 21}, {"Admin", 22}}},
		}}
		chunks := []RetrievedChunk{{ID: 21, DocID: 7, Breadcrumb: "Hub > Docs > Review needed", Metadata: titled}}
		got, err := AttachLinks(ctx, chunks, Filter{}, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		groups := got.Documents["7"].LinksOut
		if len(groups) != 3 {
			t.Fatalf("groups = %d, want 3", len(groups))
		}
		for _, g := range groups {
			wantOpen := g.Section == "Docs > Review needed"
			if (len(g.Links) > 0) != wantOpen {
				t.Errorf("group %q has %d links, want open = %v (no prefix match on the parent section)", g.Section, len(g.Links), wantOpen)
			}
			if g.Total != 4 {
				t.Errorf("group %q total = %d, want 4", g.Section, g.Total)
			}
		}
		wantIDs := map[string]int64{"Docs": 20, "Docs > Review needed": 21, "Admin": 22}
		for _, g := range groups {
			if g.ChunkID == nil || *g.ChunkID != wantIDs[g.Section] {
				t.Errorf("group %q chunk_id = %v, want %d", g.Section, g.ChunkID, wantIDs[g.Section])
			}
		}
	})

	t.Run("a hub without a matching hit section has all groups closed, a section without chunks has no chunk_id", func(t *testing.T) {
		loader := &fakeLinkLoader{data: LinkData{
			Out:      map[int64][]LinkedDoc{7: hubLinks([]string{"A", "B"}, 6)},
			Sections: map[int64][]SectionStart{7: {{"A", 30}}},
		}}
		chunks := []RetrievedChunk{{ID: 30, DocID: 7, Breadcrumb: "Other", Metadata: titled}}
		got, err := AttachLinks(ctx, chunks, Filter{}, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		want := []LinkGroup{{Section: "A", Total: 6, ChunkID: ptr(30)}, {Section: "B", Total: 6}}
		if diff := cmp.Diff(want, got.Documents["7"].LinksOut); diff != "" {
			t.Errorf("links_out (-want +got):\n%s", diff)
		}
	})

	t.Run("an open group stops at the limit, the total stays real", func(t *testing.T) {
		loader := &fakeLinkLoader{data: LinkData{Out: map[int64][]LinkedDoc{7: hubLinks([]string{"S"}, maxLinksInGroup+5)}}}
		chunks := []RetrievedChunk{{ID: 1, DocID: 7, Breadcrumb: "Hub > S", Metadata: titled}}
		got, err := AttachLinks(ctx, chunks, Filter{}, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		g := got.Documents["7"].LinksOut[0]
		if len(g.Links) != maxLinksInGroup || g.Total != maxLinksInGroup+5 {
			t.Errorf("group has %d links and total %d, want %d and %d", len(g.Links), g.Total, maxLinksInGroup, maxLinksInGroup+5)
		}
	})

	t.Run("incoming links are sorted by the shown title and stop at the limit", func(t *testing.T) {
		in := []LinkedDoc{
			{Section: "Review needed", Name: "zeta.md", Metadata: map[string]any{}, ChunkID: 3},
			{Section: "", Name: "x.md", Metadata: map[string]any{"doc.title": "Alpha"}, ChunkID: 1, Text: "see"},
		}
		for i := range maxLinksIn {
			in = append(in, LinkedDoc{Name: fmt.Sprintf("m%02d.md", i), Metadata: map[string]any{}, ChunkID: int64(100 + i)})
		}
		loader := &fakeLinkLoader{data: LinkData{In: map[int64][]LinkedDoc{7: in}}}
		chunks := []RetrievedChunk{{ID: 1, DocID: 7, Metadata: titled}}
		got, err := AttachLinks(ctx, chunks, Filter{}, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		doc := got.Documents["7"]
		if doc.LinksInTotal != maxLinksIn+2 || len(doc.LinksIn) != maxLinksIn {
			t.Fatalf("links_in = %d of total %d, want %d of %d", len(doc.LinksIn), doc.LinksInTotal, maxLinksIn, maxLinksIn+2)
		}
		if diff := cmp.Diff(LinkEntry{Title: "Alpha", Text: "see", ChunkID: 1}, doc.LinksIn[0]); diff != "" {
			t.Errorf("first incoming link (-want +got):\n%s", diff)
		}
		if got.CountLinks() != maxLinksIn {
			t.Errorf("CountLinks() = %d, want %d", got.CountLinks(), maxLinksIn)
		}
	})

	t.Run("a loader error is returned", func(t *testing.T) {
		loader := &fakeLinkLoader{err: errors.New("db gone")}
		_, err := AttachLinks(ctx, []RetrievedChunk{{ID: 1, DocID: 7}}, Filter{}, LinkDeps{Loader: loader})
		if err == nil {
			t.Fatal("AttachLinks() error = nil, want the loader error")
		}
	})

	t.Run("the JSON output uses snake_case and has no metadata on the chunks", func(t *testing.T) {
		loader := &fakeLinkLoader{data: LinkData{}}
		chunks := []RetrievedChunk{{ID: 1, DocID: 7, DocTitle: "a.md", SourceName: "wiki", Metadata: map[string]any{"k": true}}}
		got, err := AttachLinks(ctx, chunks, Filter{}, LinkDeps{Loader: loader})
		if err != nil {
			t.Fatalf("AttachLinks() error = %v", err)
		}
		raw, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		want := `{"chunks":[{"id":1,"doc_id":7,"rank":0,"doc_title":"a.md","position":0,"score":0,"breadcrumb":"","text":"","source_weight":0,"source_name":"wiki"}],` +
			`"documents":{"7":{"title":"a.md","source":"wiki","metadata":{"k":true},"links_out":[],"links_out_total":0,"links_in":[],"links_in_total":0}}}`
		if diff := cmp.Diff(want, string(raw)); diff != "" {
			t.Errorf("JSON (-want +got):\n%s", diff)
		}
	})
}

func TestChunkSection(t *testing.T) {
	tests := []struct{ breadcrumb, title, want string }{
		{"Hub > Admin", "Hub", "Admin"},
		{"Admin", "Hub", "Admin"},
		{"Hub", "Hub", ""},
		{"Hubert > Admin", "Hub", "Hubert > Admin"},
		{"Hub > Admin", "", "Hub > Admin"},
	}
	for _, tt := range tests {
		if got := chunkSection(tt.breadcrumb, tt.title); got != tt.want {
			t.Errorf("chunkSection(%q, %q) = %q, want %q", tt.breadcrumb, tt.title, got, tt.want)
		}
	}
}
