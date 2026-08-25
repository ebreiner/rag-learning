package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

var testCtx = context.Background()
var testLogger = slog.New(slog.DiscardHandler)

// drainSourceDocs calls NextSourceDoc until io.EOF and returns everything
// yielded along the way.
func drainSourceDocs(t *testing.T, s *DocSource) []stepSourceDoc {
	t.Helper()
	var docs []stepSourceDoc
	for {
		doc, err := s.NextSourceDoc()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextSourceDoc() error = %v", err)
		}
		docs = append(docs, stepSourceDoc{Name: doc.Name, SHA256: doc.SHA256, CollectionName: doc.CollectionName, CollectionWeight: doc.CollectionWeight})
	}
	return docs
}

// stepSourceDoc is a local, trimmed mirror of step.SourceDoc's fields used in
// these assertions, just to keep the test table terse.
type stepSourceDoc struct {
	Name             string
	SHA256           string
	CollectionName   string
	CollectionWeight float64
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestNewSourceDocSource(t *testing.T) {
	t.Run("yields allowed-extension files and skips unsupported ones", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.md"), "a")
		writeFile(t, filepath.Join(dir, "b.txt"), "b")
		writeFile(t, filepath.Join(dir, "c.unsupported"), "c")

		src, err := NewSourceDocSource(testCtx, dir, 1.0, "manual", testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}

		docs := drainSourceDocs(t, &src)
		if len(docs) != 2 {
			t.Fatalf("got %d docs, want 2: %+v", len(docs), docs)
		}
		names := map[string]bool{}
		for _, d := range docs {
			names[d.Name] = true
		}
		if !names["a.md"] || !names["b.txt"] {
			t.Errorf("got docs %+v, want a.md and b.txt", docs)
		}
		if names["c.unsupported"] {
			t.Errorf("c.unsupported should have been skipped")
		}
	})

	t.Run("skips dotfile directories entirely", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "visible.md"), "v")
		writeFile(t, filepath.Join(dir, ".hidden", "nested.md"), "n")

		src, err := NewSourceDocSource(testCtx, dir, 1.0, "manual", testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}

		docs := drainSourceDocs(t, &src)
		if len(docs) != 1 || docs[0].Name != "visible.md" {
			t.Fatalf("got docs %+v, want only visible.md", docs)
		}
	})

	t.Run("every yielded doc carries the same stamped collection name and weight", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.md"), "a")
		writeFile(t, filepath.Join(dir, "b.md"), "b")

		src, err := NewSourceDocSource(testCtx, dir, 0.8, "manual", testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}

		docs := drainSourceDocs(t, &src)
		if len(docs) != 2 {
			t.Fatalf("got %d docs, want 2", len(docs))
		}
		for _, d := range docs {
			if d.CollectionName != "manual" || d.CollectionWeight != 0.8 {
				t.Errorf("doc %q collection = (%q, %v), want (%q, %v)", d.Name, d.CollectionName, d.CollectionWeight, "manual", 0.8)
			}
		}
	})

	t.Run("sha256 matches the file's actual content", func(t *testing.T) {
		dir := t.TempDir()
		content := "known content for hashing"
		writeFile(t, filepath.Join(dir, "a.md"), content)

		src, err := NewSourceDocSource(testCtx, dir, 1.0, "manual", testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}

		doc, err := src.NextSourceDoc()
		if err != nil {
			t.Fatalf("NextSourceDoc() error = %v", err)
		}

		sum := sha256.Sum256([]byte(content))
		want := hex.EncodeToString(sum[:])
		if doc.SHA256 != want {
			t.Errorf("SHA256 = %q, want %q", doc.SHA256, want)
		}
	})

	t.Run("NextSourceDoc returns io.EOF once exhausted", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.md"), "a")

		src, err := NewSourceDocSource(testCtx, dir, 1.0, "manual", testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}

		if _, err := src.NextSourceDoc(); err != nil {
			t.Fatalf("first NextSourceDoc() error = %v, want nil", err)
		}
		if _, err := src.NextSourceDoc(); err != io.EOF {
			t.Fatalf("second NextSourceDoc() error = %v, want io.EOF", err)
		}
	})
}
