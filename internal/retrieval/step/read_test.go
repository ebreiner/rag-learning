package step

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type fakeLoader struct {
	doc        LoadedDocument
	err        error
	gotFilter  Filter
	gotChunkID int64
}

func (f *fakeLoader) DocumentOfChunk(ctx context.Context, chunkID int64, filter Filter) (LoadedDocument, error) {
	f.gotChunkID, f.gotFilter = chunkID, filter
	return f.doc, f.err
}

// docChunk builds a chunk like the chunker: breadcrumb + "\n\n" + body.
func docChunk(id int64, breadcrumb, body string) DocChunk {
	return DocChunk{ID: id, Position: id, Breadcrumb: breadcrumb, Text: breadcrumb + "\n\n" + body}
}

func body200(label string) string {
	return label + " " + strings.Repeat("x", 200-len(label)-1)
}

// testDoc has a section "Setup" with 2 sub-sections, and "Setupx", which only starts with the same letters.
func testDoc(bodyOf func(string) string) LoadedDocument {
	return LoadedDocument{Name: "doc.md", Source: "manual", Metadata: map[string]any{"k": true}, Chunks: []DocChunk{
		docChunk(1, "Intro", bodyOf("intro")),
		docChunk(2, "Setup", bodyOf("setup")),
		docChunk(3, "Setup > Step 1", bodyOf("one")),
		docChunk(4, "Setup > Step 1", bodyOf("one more")),
		docChunk(5, "Setup > Step 2", bodyOf("two")),
		docChunk(6, "Setupx", bodyOf("setupx")),
		docChunk(7, "End", bodyOf("end")),
	}}
}

func short(label string) string { return label }

func readDeps(doc LoadedDocument, maxChars int64) (ReadDeps, *fakeLoader) {
	loader := &fakeLoader{doc: doc}
	return ReadDeps{Loader: loader, MaxChars: maxChars, Sources: []string{"manual"}}, loader
}

func TestRunReadModes(t *testing.T) {
	tests := []struct {
		name string
		req  ReadRequest
		want string
	}{
		{
			name: "neighbours at the start of the document, default n is 2",
			req:  ReadRequest{ChunkID: 1, Mode: ReadNeighbours},
			want: "## Intro\n\nintro\n\n## Setup\n\nsetup\n\n## Setup > Step 1\n\none",
		},
		{
			name: "neighbours at the end of the document with n 1",
			req:  ReadRequest{ChunkID: 7, Mode: ReadNeighbours, N: 1},
			want: "## Setupx\n\nsetupx\n\n## End\n\nend",
		},
		{
			name: "section with its sub-sections, not a breadcrumb that only starts with the same letters",
			req:  ReadRequest{ChunkID: 2, Mode: ReadSection},
			want: "## Setup\n\nsetup\n\n## Setup > Step 1\n\none\n\none more\n\n## Setup > Step 2\n\ntwo",
		},
		{
			name: "section of a sub-section, each breadcrumb once",
			req:  ReadRequest{ChunkID: 4, Mode: ReadSection},
			want: "## Setup > Step 1\n\none\n\none more",
		},
		{
			name: "document",
			req:  ReadRequest{ChunkID: 5, Mode: ReadDocument},
			want: "## Intro\n\nintro\n\n## Setup\n\nsetup\n\n## Setup > Step 1\n\none\n\none more\n\n## Setup > Step 2\n\ntwo\n\n## Setupx\n\nsetupx\n\n## End\n\nend",
		},
		{
			name: "page from a chunk to the end",
			req:  ReadRequest{ChunkID: 6, Mode: ReadPage},
			want: "## Setupx\n\nsetupx\n\n## End\n\nend",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _ := readDeps(testDoc(short), 30000)
			got, err := RunRead(testCtx, tt.req, deps)
			if err != nil {
				t.Fatalf("RunRead() error = %v", err)
			}
			if diff := cmp.Diff(tt.want, got.Text); diff != "" {
				t.Errorf("text (-want +got):\n%s", diff)
			}
			if got.TooBig || got.Outline != nil || got.NextChunkID != nil || got.Warning != "" {
				t.Errorf("result = %+v, want text only", got)
			}
			if got.Document != "doc.md" || got.Source != "manual" || got.Metadata["k"] != true || got.Chars != len(tt.want) {
				t.Errorf("result fields = %+v, want document, source, metadata and chars", got)
			}
		})
	}
}

func TestRunReadLimit(t *testing.T) {
	t.Run("a document over the limit gives its outline", func(t *testing.T) {
		deps, _ := readDeps(testDoc(body200), 1000)
		got, err := RunRead(testCtx, ReadRequest{ChunkID: 1, Mode: ReadDocument}, deps)
		if err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		want := []OutlineEntry{
			{Breadcrumb: "Intro", Chars: 200, ChunkID: 1},
			{Breadcrumb: "Setup", Chars: 200, ChunkID: 2},
			{Breadcrumb: "Setup > Step 1", Chars: 400, ChunkID: 3},
			{Breadcrumb: "Setup > Step 2", Chars: 200, ChunkID: 5},
			{Breadcrumb: "Setupx", Chars: 200, ChunkID: 6},
			{Breadcrumb: "End", Chars: 200, ChunkID: 7},
		}
		if !got.TooBig || got.Text != "" {
			t.Errorf("result = %+v, want too_big and no text", got)
		}
		if diff := cmp.Diff(want, got.Outline); diff != "" {
			t.Errorf("outline (-want +got):\n%s", diff)
		}
	})

	t.Run("a section over the limit gives the outline of its sub-sections", func(t *testing.T) {
		deps, _ := readDeps(testDoc(body200), 500)
		got, err := RunRead(testCtx, ReadRequest{ChunkID: 2, Mode: ReadSection}, deps)
		if err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		want := []OutlineEntry{
			{Breadcrumb: "Setup", Chars: 200, ChunkID: 2},
			{Breadcrumb: "Setup > Step 1", Chars: 400, ChunkID: 3},
			{Breadcrumb: "Setup > Step 2", Chars: 200, ChunkID: 5},
		}
		if diff := cmp.Diff(want, got.Outline); diff != "" {
			t.Errorf("outline (-want +got):\n%s", diff)
		}
	})

	t.Run("an outline with one entry gives pages, and page continues to the last page", func(t *testing.T) {
		flat := LoadedDocument{Name: "flat.pdf", Source: "manual"}
		for i := int64(1); i <= 5; i++ {
			flat.Chunks = append(flat.Chunks, docChunk(i, "Vertrag", body200(fmt.Sprintf("c%d", i))))
		}
		deps, _ := readDeps(flat, 450)

		first, err := RunRead(testCtx, ReadRequest{ChunkID: 3, Mode: ReadDocument}, deps)
		if err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		if first.NextChunkID == nil || *first.NextChunkID != 3 || !strings.Contains(first.Text, "c2 ") || strings.Contains(first.Text, "c3 ") {
			t.Fatalf("first page = %+v, want chunks 1 and 2 and next_chunk_id 3", first)
		}

		second, err := RunRead(testCtx, ReadRequest{ChunkID: 3, Mode: ReadPage}, deps)
		if err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		if second.NextChunkID == nil || *second.NextChunkID != 5 {
			t.Fatalf("second page next_chunk_id = %v, want 5", second.NextChunkID)
		}

		last, err := RunRead(testCtx, ReadRequest{ChunkID: 5, Mode: ReadPage}, deps)
		if err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		if last.NextChunkID != nil || !strings.Contains(last.Text, "c5 ") || last.Chars > 450 {
			t.Errorf("last page = %+v, want chunk 5, no next_chunk_id, not over the limit", last)
		}
	})

	t.Run("an outline larger than the limit gives a page", func(t *testing.T) {
		deps, _ := readDeps(testDoc(body200), 250)
		got, err := RunRead(testCtx, ReadRequest{ChunkID: 1, Mode: ReadDocument}, deps)
		if err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		if got.Outline != nil || got.NextChunkID == nil || *got.NextChunkID != 2 {
			t.Errorf("result = %+v, want a page of chunk 1 with next_chunk_id 2", got)
		}
	})

	t.Run("one chunk larger than the limit gives a warning", func(t *testing.T) {
		deps, _ := readDeps(testDoc(body200), 100)
		got, err := RunRead(testCtx, ReadRequest{ChunkID: 1, Mode: ReadPage}, deps)
		if err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		if got.Warning == "" || got.Text != "" || !got.TooBig {
			t.Errorf("result = %+v, want a warning and no text", got)
		}
	})
}

func TestRunReadErrors(t *testing.T) {
	tests := []struct {
		name    string
		req     ReadRequest
		loadErr error
		want    error
	}{
		{name: "unknown mode", req: ReadRequest{ChunkID: 1, Mode: "everything"}, want: ErrInvalidRead},
		{name: "n above the maximum", req: ReadRequest{ChunkID: 1, Mode: ReadNeighbours, N: 6}, want: ErrInvalidRead},
		{name: "negative n", req: ReadRequest{ChunkID: 1, Mode: ReadNeighbours, N: -1}, want: ErrInvalidRead},
		{name: "unknown source in the filter", req: ReadRequest{ChunkID: 1, Mode: ReadSection, Filter: Filter{Sources: []string{"nope"}}}, want: ErrInvalidFilter},
		{name: "the loader excludes the chunk", req: ReadRequest{ChunkID: 1, Mode: ReadSection}, loadErr: ErrChunkNotInFilter, want: ErrChunkNotInFilter},
		{name: "the loader does not find the chunk", req: ReadRequest{ChunkID: 99, Mode: ReadSection}, loadErr: ErrChunkNotFound, want: ErrChunkNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, loader := readDeps(testDoc(short), 30000)
			loader.err = tt.loadErr
			if _, err := RunRead(testCtx, tt.req, deps); !errors.Is(err, tt.want) {
				t.Errorf("RunRead() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("the filter goes to the loader", func(t *testing.T) {
		deps, loader := readDeps(testDoc(short), 30000)
		filter := Filter{Sources: []string{"manual"}, Metadata: map[string]any{"k": true}}
		if _, err := RunRead(testCtx, ReadRequest{ChunkID: 3, Mode: ReadSection, Filter: filter}, deps); err != nil {
			t.Fatalf("RunRead() error = %v", err)
		}
		if diff := cmp.Diff(filter, loader.gotFilter); diff != "" || loader.gotChunkID != 3 {
			t.Errorf("loader got chunk %d, filter (-want +got):\n%s", loader.gotChunkID, diff)
		}
	})
}
