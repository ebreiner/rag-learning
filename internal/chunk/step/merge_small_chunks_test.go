package step

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestMergeSmallChunks(t *testing.T) {
	tests := []struct {
		name      string
		chunks    []ChunkToSave
		threshold int
		want      []ChunkToSave
	}{
		{
			name: "small content chunk merges into the next content chunk",
			chunks: []ChunkToSave{
				{Text: "tiny", Breadcrumb: "Ch1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 1}}},
				{Text: "a much longer chunk of real content here", Breadcrumb: "Ch2", Position: 1, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 2}}},
			},
			threshold: 10,
			want: []ChunkToSave{
				{Text: "tiny\n\na much longer chunk of real content here", Breadcrumb: "Ch2", Position: 1, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 1}, {ExtractionNodeID: 2, Position: 1}}},
			},
		},
		{
			// receiving chunk keeps its own Breadcrumb/Position -- the
			// absorbed chunk was already too small to represent its own
			// section meaningfully.
			name: "merged chunk's breadcrumb and position come from the receiving (second) chunk, not the absorbed one",
			chunks: []ChunkToSave{
				{Text: "x", Breadcrumb: "AbsorbedSection", Position: 5, Type: TypeContent},
				{Text: "y that is long enough to clear the threshold on its own", Breadcrumb: "ReceivingSection", Position: 6, Type: TypeContent},
			},
			threshold: 5,
			want: []ChunkToSave{
				{Text: "x\n\ny that is long enough to clear the threshold on its own", Breadcrumb: "ReceivingSection", Position: 6, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{}},
			},
		},
		{
			// a container is never touched, whether it's the small candidate
			// itself (never initiates a merge) or the next chunk (a small
			// content chunk must not be absorbed into a following container).
			name: "container-type chunks are never merged, in either direction",
			chunks: []ChunkToSave{
				{Text: "x", Breadcrumb: "Ch1", Position: 0, Type: TypeTable,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 1}}},
				{Text: "y", Breadcrumb: "Ch1", Position: 1, Type: TypeContent},
				{Text: "z", Breadcrumb: "Ch1", Position: 2, Type: TypeGeneric,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 3}}},
			},
			threshold: 10,
			want: []ChunkToSave{
				{Text: "x", Breadcrumb: "Ch1", Position: 0, Type: TypeTable,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 1}}},
				{Text: "y", Breadcrumb: "Ch1", Position: 1, Type: TypeContent},
				{Text: "z", Breadcrumb: "Ch1", Position: 2, Type: TypeGeneric,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 3}}},
			},
		},
		{
			name: "the last chunk in a document with no next chunk stays unmerged, even if small",
			chunks: []ChunkToSave{
				{Text: "all alone", Breadcrumb: "Ch1", Position: 0, Type: TypeContent},
			},
			threshold: 100,
			want: []ChunkToSave{
				{Text: "all alone", Breadcrumb: "Ch1", Position: 0, Type: TypeContent},
			},
		},
		{
			// cascade: the first merge result is still under threshold, so it
			// merges once more with the third chunk -- confirmed against
			// real corpus data that a run never exceeds 2 consecutive small
			// chunks, so a single extra check (not a general loop) suffices.
			name: "two consecutive small chunks cascade into a single merge with the third",
			chunks: []ChunkToSave{
				{Text: "a", Breadcrumb: "C1", Position: 0, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 1}}},
				{Text: "b", Breadcrumb: "C2", Position: 1, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 2}}},
				{Text: "c", Breadcrumb: "C3", Position: 2, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{{ExtractionNodeID: 3}}},
			},
			threshold: 10,
			want: []ChunkToSave{
				{Text: "a\n\nb\n\nc", Breadcrumb: "C3", Position: 2, Type: TypeContent,
					ExtractionNodeIDs: []ChunkExtractionNodeID{
						{ExtractionNodeID: 1},
						{ExtractionNodeID: 2, Position: 1},
						{ExtractionNodeID: 3, Position: 2},
					}},
			},
		},
		{
			name: "a content chunk at or above threshold is left alone regardless of its neighbors",
			chunks: []ChunkToSave{
				{Text: "already long enough on its own to clear the threshold", Breadcrumb: "Ch1", Position: 0, Type: TypeContent},
				{Text: "tiny", Breadcrumb: "Ch2", Position: 1, Type: TypeContent},
			},
			threshold: 10,
			want: []ChunkToSave{
				{Text: "already long enough on its own to clear the threshold", Breadcrumb: "Ch1", Position: 0, Type: TypeContent},
				{Text: "tiny", Breadcrumb: "Ch2", Position: 1, Type: TypeContent},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeSmallChunks(tt.chunks, tt.threshold)
			if diff := cmp.Diff(tt.want, got, cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("mergeSmallChunks() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
