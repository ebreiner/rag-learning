package step

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var (
	testCtx    = context.Background()
	testLogger = slog.New(slog.DiscardHandler)
)

// fakeSource returns one batch per call from a preconfigured queue, and
// records every limit it was called with.
type fakeSource struct {
	batches      [][]ChunkToEmbed
	call         int
	limitsCalled []int64
	err          error
}

func (f *fakeSource) NextChunks(limit int64, ctx context.Context) ([]ChunkToEmbed, error) {
	f.limitsCalled = append(f.limitsCalled, limit)
	if f.err != nil {
		return nil, f.err
	}
	if f.call >= len(f.batches) {
		return nil, nil
	}
	batch := f.batches[f.call]
	f.call++
	return batch, nil
}

// fakeClient records every batch of chunks it was asked to embed.
type fakeClient struct {
	calledWith [][]ChunkToEmbed
	err        error
}

func (f *fakeClient) EmbedChunks(chunks []ChunkToEmbed, ctx context.Context) (EmbeddingsToSave, error) {
	f.calledWith = append(f.calledWith, chunks)
	if f.err != nil {
		return EmbeddingsToSave{}, f.err
	}
	embeddings := make([]Embedding, len(chunks))
	for i, c := range chunks {
		embeddings[i] = Embedding{ChunkID: c.ChunkID, Vector: []float64{0.1, 0.2}}
	}
	return EmbeddingsToSave{Model: "fake", Dim: 2, Embeddings: embeddings}, nil
}

// fakeSink records every EmbeddingsToSave it was asked to persist.
type fakeSink struct {
	saved []EmbeddingsToSave
	err   error
}

func (f *fakeSink) SaveEmbeddings(toSave EmbeddingsToSave, ctx context.Context) error {
	f.saved = append(f.saved, toSave)
	return f.err
}

// flakyClient simulates the two real failure modes embedWithFallback exists
// to survive: a batch that's "too large" (mirrors Ollama's real batch-NaN
// bug, where several chunks together break the batch even though each is
// fine alone), and a specific chunk that's permanently unembeddable
// regardless of batch size.
type flakyClient struct {
	failIfBatchLargerThan int
	permanentlyBadChunkID int64
	calls                 [][]ChunkToEmbed
}

func (f *flakyClient) EmbedChunks(chunks []ChunkToEmbed, ctx context.Context) (EmbeddingsToSave, error) {
	f.calls = append(f.calls, chunks)

	for _, c := range chunks {
		if c.ChunkID == f.permanentlyBadChunkID {
			return EmbeddingsToSave{}, errors.New("simulated permanent failure")
		}
	}
	if f.failIfBatchLargerThan > 0 && len(chunks) > f.failIfBatchLargerThan {
		return EmbeddingsToSave{}, errors.New("simulated batch-too-large failure")
	}

	embeddings := make([]Embedding, len(chunks))
	for i, c := range chunks {
		embeddings[i] = Embedding{ChunkID: c.ChunkID, Vector: []float64{0.1, 0.2}}
	}
	return EmbeddingsToSave{Model: "fake", Dim: 2, Embeddings: embeddings}, nil
}

func TestEmbedWithFallback(t *testing.T) {
	t.Run("succeeds directly, no split, when the whole batch embeds fine", func(t *testing.T) {
		client := &flakyClient{}
		chunks := []ChunkToEmbed{{ChunkID: 1, Text: "a"}, {ChunkID: 2, Text: "b"}}

		got, err := embedWithFallback(client, chunks, testCtx, testLogger)
		if err != nil {
			t.Fatalf("embedWithFallback() error = %v", err)
		}
		if len(got.Embeddings) != 2 {
			t.Fatalf("got %d embeddings, want 2", len(got.Embeddings))
		}
		if len(client.calls) != 1 {
			t.Fatalf("client called %d times, want 1 (no split needed)", len(client.calls))
		}
	})

	t.Run("a batch that fails together but succeeds once split small enough recovers everything", func(t *testing.T) {
		client := &flakyClient{failIfBatchLargerThan: 2}
		chunks := []ChunkToEmbed{
			{ChunkID: 1, Text: "a"}, {ChunkID: 2, Text: "b"},
			{ChunkID: 3, Text: "c"}, {ChunkID: 4, Text: "d"},
		}

		got, err := embedWithFallback(client, chunks, testCtx, testLogger)
		if err != nil {
			t.Fatalf("embedWithFallback() error = %v", err)
		}
		if len(got.Embeddings) != 4 {
			t.Fatalf("got %d embeddings, want 4 -- all chunks should eventually succeed once split small enough", len(got.Embeddings))
		}
	})

	t.Run("a chunk that fails even alone is skipped without blocking its siblings", func(t *testing.T) {
		client := &flakyClient{permanentlyBadChunkID: 2}
		chunks := []ChunkToEmbed{
			{ChunkID: 1, Text: "a"}, {ChunkID: 2, Text: "b"}, {ChunkID: 3, Text: "c"},
		}

		got, err := embedWithFallback(client, chunks, testCtx, testLogger)
		if err != nil {
			t.Fatalf("embedWithFallback() error = %v", err)
		}
		gotIDs := make([]int64, 0, len(got.Embeddings))
		for _, e := range got.Embeddings {
			gotIDs = append(gotIDs, e.ChunkID)
		}
		want := []int64{1, 3}
		if diff := cmp.Diff(want, gotIDs); diff != "" {
			t.Errorf("embedded chunk ids mismatch (-want +got):\n%s", diff)
		}
	})
}

func TestEmbed(t *testing.T) {
	t.Run("single small batch processes once and exits", func(t *testing.T) {
		source := &fakeSource{batches: [][]ChunkToEmbed{
			{{ChunkID: 1, Text: "hello world"}, {ChunkID: 2, Text: "goodbye world"}},
		}}
		client := &fakeClient{}
		sink := &fakeSink{}

		if err := Embed(sink, source, client, testCtx, testLogger); err != nil {
			t.Fatalf("Embed() error = %v", err)
		}

		if len(client.calledWith) != 1 {
			t.Fatalf("client called %d times, want 1", len(client.calledWith))
		}
		if len(client.calledWith[0]) != 2 {
			t.Fatalf("client got %d chunks, want 2", len(client.calledWith[0]))
		}
		if len(sink.saved) != 1 {
			t.Fatalf("sink called %d times, want 1", len(sink.saved))
		}
	})

	t.Run("multiple full batches keep looping until a short batch ends it", func(t *testing.T) {
		full := func(n int) []ChunkToEmbed {
			batch := make([]ChunkToEmbed, n)
			for i := range batch {
				batch[i] = ChunkToEmbed{ChunkID: int64(i + 1), Text: "some real text here"}
			}
			return batch
		}
		source := &fakeSource{batches: [][]ChunkToEmbed{
			full(10), // == limit, loop must continue
			full(10), // == limit, loop must continue
			full(3),  // < limit, loop must stop after this one
		}}
		client := &fakeClient{}
		sink := &fakeSink{}

		if err := Embed(sink, source, client, testCtx, testLogger); err != nil {
			t.Fatalf("Embed() error = %v", err)
		}

		if len(client.calledWith) != 3 {
			t.Fatalf("client called %d times, want 3 (loop should stop after the short batch)", len(client.calledWith))
		}
		if len(sink.saved) != 3 {
			t.Fatalf("sink called %d times, want 3", len(sink.saved))
		}
	})

	t.Run("chunks with near-empty text are filtered before reaching the client", func(t *testing.T) {
		source := &fakeSource{batches: [][]ChunkToEmbed{
			{
				{ChunkID: 1, Text: ""},
				{ChunkID: 2, Text: "a"},
				{ChunkID: 3, Text: "a real chunk of text"},
			},
		}}
		client := &fakeClient{}
		sink := &fakeSink{}

		if err := Embed(sink, source, client, testCtx, testLogger); err != nil {
			t.Fatalf("Embed() error = %v", err)
		}

		if len(client.calledWith) != 1 {
			t.Fatalf("client called %d times, want 1", len(client.calledWith))
		}
		got := make([]int64, 0)
		for _, c := range client.calledWith[0] {
			got = append(got, c.ChunkID)
		}
		want := []int64{3}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("filtered chunk ids mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("zero chunks on the first call is a clean exit, not an error", func(t *testing.T) {
		source := &fakeSource{batches: [][]ChunkToEmbed{}}
		client := &fakeClient{}
		sink := &fakeSink{}

		if err := Embed(sink, source, client, testCtx, testLogger); err != nil {
			t.Fatalf("Embed() error = %v, want nil", err)
		}
		if len(client.calledWith) != 0 {
			t.Fatalf("client should never have been called, was called %d times", len(client.calledWith))
		}
	})

	t.Run("source error halts the loop and propagates", func(t *testing.T) {
		wantErr := errors.New("source broke")
		source := &fakeSource{err: wantErr}
		client := &fakeClient{}
		sink := &fakeSink{}

		err := Embed(sink, source, client, testCtx, testLogger)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Embed() error = %v, want %v", err, wantErr)
		}
		if len(client.calledWith) != 0 {
			t.Fatalf("client should never have been called after a source error")
		}
	})

	t.Run("a client that fails for every chunk logs and skips them instead of halting the run", func(t *testing.T) {
		// embedWithFallback's base case treats a chunk that fails even alone
		// as an individual, non-fatal problem -- this is the intended
		// behavior added to survive Ollama's real batch-NaN bug, not a bug
		// in Embed() itself.
		source := &fakeSource{batches: [][]ChunkToEmbed{
			{{ChunkID: 1, Text: "a real chunk of text"}},
		}}
		client := &fakeClient{err: errors.New("embed call broke")}
		sink := &fakeSink{}

		if err := Embed(sink, source, client, testCtx, testLogger); err != nil {
			t.Fatalf("Embed() error = %v, want nil -- an unembeddable chunk should be skipped, not fatal", err)
		}

		if len(sink.saved) != 0 {
			t.Fatalf("sink called %d times, want 0 -- nothing embeddable in the batch, SaveEmbeddings shouldn't be called at all", len(sink.saved))
		}
	})

	t.Run("sink error halts the loop and propagates", func(t *testing.T) {
		wantErr := errors.New("save broke")
		source := &fakeSource{batches: [][]ChunkToEmbed{
			{{ChunkID: 1, Text: "a real chunk of text"}},
		}}
		client := &fakeClient{}
		sink := &fakeSink{err: wantErr}

		err := Embed(sink, source, client, testCtx, testLogger)
		if !errors.Is(err, wantErr) {
			t.Fatalf("Embed() error = %v, want %v", err, wantErr)
		}
	})
}
