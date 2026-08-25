package step

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

var testCtx = context.Background()
var testLogger = slog.New(slog.DiscardHandler)

type existsResult struct {
	isDuplicate    bool
	collectionName string
}

type fakeDocSource struct {
	docs []SourceDoc
	idx  int
	err  error // returned once docs are exhausted; nil means io.EOF
}

func (f *fakeDocSource) NextSourceDoc() (SourceDoc, error) {
	if f.idx >= len(f.docs) {
		if f.err != nil {
			return SourceDoc{}, f.err
		}
		return SourceDoc{}, io.EOF
	}
	doc := f.docs[f.idx]
	f.idx++
	return doc, nil
}

type fakeDocSink struct {
	existsResults map[string]existsResult
	existsErr     error
	existsCalls   []string

	saveErr   error
	savedDocs []ExtractedDoc
	nextID    int64
}

func (f *fakeDocSink) ExistsDoc(ctx context.Context, sha256 string) (bool, string, error) {
	f.existsCalls = append(f.existsCalls, sha256)
	if f.existsErr != nil {
		return false, "", f.existsErr
	}
	if res, ok := f.existsResults[sha256]; ok {
		return res.isDuplicate, res.collectionName, nil
	}
	return false, "", nil
}

func (f *fakeDocSink) SaveExtractedDoc(ctx context.Context, doc ExtractedDoc) (int64, error) {
	if f.saveErr != nil {
		return -1, f.saveErr
	}
	f.savedDocs = append(f.savedDocs, doc)
	f.nextID++
	return f.nextID, nil
}

type fakeExtractor struct {
	err          error
	extractCalls []SourceDoc
}

func (f *fakeExtractor) ExtractSourceDoc(ctx context.Context, doc SourceDoc) (ExtractedDoc, error) {
	f.extractCalls = append(f.extractCalls, doc)
	if f.err != nil {
		return ExtractedDoc{}, f.err
	}
	// mirrors docling.DoclingExtractor.ExtractSourceDoc: the source doc is
	// always carried through onto the extracted doc's Source field.
	return ExtractedDoc{Source: doc}, nil
}

func TestExtract(t *testing.T) {
	t.Run("missing sha256 wraps ErrExtractionFailed without touching the sink or extractor", func(t *testing.T) {
		source := &fakeDocSource{docs: []SourceDoc{{Name: "doc", SHA256: ""}}}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{}

		err := extract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, ErrExtractionFailed) {
			t.Fatalf("extract() error = %v, want ErrExtractionFailed", err)
		}
		if len(sink.existsCalls) != 0 {
			t.Errorf("sink.ExistsDoc should not have been called")
		}
		if len(extractor.extractCalls) != 0 {
			t.Errorf("extractor should not have been called")
		}
	})

	t.Run("EOF from the source propagates as io.EOF", func(t *testing.T) {
		source := &fakeDocSource{}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{}

		err := extract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, io.EOF) {
			t.Fatalf("extract() error = %v, want io.EOF", err)
		}
	})

	t.Run("a non-EOF source error wraps ErrExtractionFailed", func(t *testing.T) {
		wantErr := errors.New("disk broke")
		source := &fakeDocSource{err: wantErr}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{}

		err := extract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, ErrExtractionFailed) || !errors.Is(err, wantErr) {
			t.Fatalf("extract() error = %v, want wrapping both ErrExtractionFailed and %v", err, wantErr)
		}
	})

	t.Run("duplicate doc under the same collection is skipped without saving", func(t *testing.T) {
		doc := SourceDoc{SHA256: "abc", Name: "doc", CollectionName: "manual"}
		source := &fakeDocSource{docs: []SourceDoc{doc}}
		sink := &fakeDocSink{existsResults: map[string]existsResult{
			"abc": {isDuplicate: true, collectionName: "manual"},
		}}
		extractor := &fakeExtractor{}

		err := extract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, ErrDuplicateDoc) {
			t.Fatalf("extract() error = %v, want ErrDuplicateDoc", err)
		}
		if len(extractor.extractCalls) != 0 {
			t.Errorf("extractor should not have been called for a duplicate")
		}
		if len(sink.savedDocs) != 0 {
			t.Errorf("sink.SaveExtractedDoc should not have been called for a duplicate")
		}
	})

	t.Run("duplicate doc re-submitted under a different collection is still skipped, not re-tagged", func(t *testing.T) {
		doc := SourceDoc{SHA256: "abc", Name: "doc", CollectionName: "changelog"}
		source := &fakeDocSource{docs: []SourceDoc{doc}}
		sink := &fakeDocSink{existsResults: map[string]existsResult{
			"abc": {isDuplicate: true, collectionName: "manual"},
		}}
		extractor := &fakeExtractor{}

		err := extract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, ErrDuplicateDoc) {
			t.Fatalf("extract() error = %v, want ErrDuplicateDoc", err)
		}
		if len(sink.savedDocs) != 0 {
			t.Errorf("sink.SaveExtractedDoc should not have been called -- a doc's collection must not change via re-extraction")
		}
	})

	t.Run("collection name and weight propagate from the source doc to the saved extracted doc", func(t *testing.T) {
		doc := SourceDoc{SHA256: "abc", Name: "doc", CollectionName: "manual", CollectionWeight: 0.8}
		source := &fakeDocSource{docs: []SourceDoc{doc}}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{}

		if err := extract(testCtx, source, sink, extractor, testLogger); err != nil {
			t.Fatalf("extract() error = %v", err)
		}
		if len(sink.savedDocs) != 1 {
			t.Fatalf("got %d saved docs, want 1", len(sink.savedDocs))
		}
		got := sink.savedDocs[0].Source
		if got.CollectionName != "manual" || got.CollectionWeight != 0.8 {
			t.Errorf("saved doc collection = (%q, %v), want (%q, %v)", got.CollectionName, got.CollectionWeight, "manual", 0.8)
		}
	})

	t.Run("extractor error wraps ErrExtractionFailed", func(t *testing.T) {
		wantErr := errors.New("docling broke")
		doc := SourceDoc{SHA256: "abc", Name: "doc"}
		source := &fakeDocSource{docs: []SourceDoc{doc}}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{err: wantErr}

		err := extract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, ErrExtractionFailed) || !errors.Is(err, wantErr) {
			t.Fatalf("extract() error = %v, want wrapping both ErrExtractionFailed and %v", err, wantErr)
		}
	})

	t.Run("sink save error propagates unwrapped, unlike an extractor error", func(t *testing.T) {
		wantErr := errors.New("disk full")
		doc := SourceDoc{SHA256: "abc", Name: "doc"}
		source := &fakeDocSource{docs: []SourceDoc{doc}}
		sink := &fakeDocSink{saveErr: wantErr}
		extractor := &fakeExtractor{}

		err := extract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, wantErr) {
			t.Fatalf("extract() error = %v, want %v", err, wantErr)
		}
		if errors.Is(err, ErrExtractionFailed) {
			t.Errorf("sink save errors should not be wrapped as ErrExtractionFailed -- RunExtract treats that as soft/retryable, a save error should stop the loop immediately")
		}
	})
}

func TestRunExtract(t *testing.T) {
	t.Run("immediate EOF calls neither the extractor nor the sink", func(t *testing.T) {
		source := &fakeDocSource{}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{}

		if err := RunExtract(testCtx, source, sink, extractor, testLogger); err != nil {
			t.Fatalf("RunExtract() error = %v", err)
		}
		if len(extractor.extractCalls) != 0 || len(sink.savedDocs) != 0 {
			t.Errorf("extractor/sink should not have been called")
		}
	})

	t.Run("N docs are each extracted and saved exactly once", func(t *testing.T) {
		docs := []SourceDoc{
			{SHA256: "a", Name: "doc-a"},
			{SHA256: "b", Name: "doc-b"},
			{SHA256: "c", Name: "doc-c"},
		}
		source := &fakeDocSource{docs: docs}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{}

		if err := RunExtract(testCtx, source, sink, extractor, testLogger); err != nil {
			t.Fatalf("RunExtract() error = %v", err)
		}
		if len(extractor.extractCalls) != 3 {
			t.Fatalf("extractor called %d times, want 3", len(extractor.extractCalls))
		}
		if len(sink.savedDocs) != 3 {
			t.Fatalf("sink called %d times, want 3", len(sink.savedDocs))
		}
	})

	t.Run("a duplicate doc is skipped without stopping the loop", func(t *testing.T) {
		docs := []SourceDoc{
			{SHA256: "a", Name: "doc-a"},
			{SHA256: "dup", Name: "doc-dup"},
			{SHA256: "c", Name: "doc-c"},
		}
		source := &fakeDocSource{docs: docs}
		sink := &fakeDocSink{existsResults: map[string]existsResult{
			"dup": {isDuplicate: true, collectionName: ""},
		}}
		extractor := &fakeExtractor{}

		if err := RunExtract(testCtx, source, sink, extractor, testLogger); err != nil {
			t.Fatalf("RunExtract() error = %v", err)
		}
		if len(sink.savedDocs) != 2 {
			t.Fatalf("sink called %d times, want 2 (dup skipped)", len(sink.savedDocs))
		}
	})

	t.Run("sink save error stops the loop immediately, unlike an extractor error", func(t *testing.T) {
		docs := []SourceDoc{
			{SHA256: "a", Name: "doc-a"},
			{SHA256: "b", Name: "doc-b"},
		}
		wantErr := errors.New("disk full")
		source := &fakeDocSource{docs: docs}
		sink := &fakeDocSink{saveErr: wantErr}
		extractor := &fakeExtractor{}

		err := RunExtract(testCtx, source, sink, extractor, testLogger)
		if !errors.Is(err, wantErr) {
			t.Fatalf("RunExtract() error = %v, want %v", err, wantErr)
		}
		if len(extractor.extractCalls) != 1 {
			t.Errorf("extractor called %d times, want 1 (loop should stop on the first sink error)", len(extractor.extractCalls))
		}
	})

	t.Run("extractor error does not stop the loop until 10 consecutive failures", func(t *testing.T) {
		docs := make([]SourceDoc, 11)
		for i := range docs {
			docs[i] = SourceDoc{SHA256: string(rune('a' + i)), Name: "doc"}
		}
		source := &fakeDocSource{docs: docs}
		sink := &fakeDocSink{}
		extractor := &fakeExtractor{err: errors.New("docling broke")}

		err := RunExtract(testCtx, source, sink, extractor, testLogger)
		if err == nil {
			t.Fatalf("RunExtract() error = nil, want an error after 10 consecutive failures")
		}
		if len(extractor.extractCalls) != 10 {
			t.Errorf("extractor called %d times, want exactly 10 (stops at the breaker, never reaches the 11th doc)", len(extractor.extractCalls))
		}
	})

	t.Run("a success in between resets the consecutive failure count", func(t *testing.T) {
		docs := make([]SourceDoc, 19)
		for i := range docs {
			docs[i] = SourceDoc{SHA256: string(rune('a' + i)), Name: "doc"}
		}
		source := &fakeDocSource{docs: docs}
		sink := &fakeDocSink{}
		callCount := 0
		extractor := &fakeExtractorFunc{fn: func(doc SourceDoc) (ExtractedDoc, error) {
			callCount++
			if doc.SHA256 == string(rune('a'+9)) {
				return ExtractedDoc{Source: doc}, nil
			}
			return ExtractedDoc{}, errors.New("docling broke")
		}}

		if err := RunExtract(testCtx, source, sink, extractor, testLogger); err != nil {
			t.Fatalf("RunExtract() error = %v, want nil -- no run of 10 consecutive failures should occur", err)
		}
		if callCount != 19 {
			t.Errorf("extractor called %d times, want 19 (all docs processed)", callCount)
		}
	})
}

type fakeExtractorFunc struct {
	fn func(doc SourceDoc) (ExtractedDoc, error)
}

func (f *fakeExtractorFunc) ExtractSourceDoc(ctx context.Context, doc SourceDoc) (ExtractedDoc, error) {
	return f.fn(doc)
}
