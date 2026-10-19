package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"rag/internal/platform/config"
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
		docs = append(docs, stepSourceDoc{Name: doc.Name, SHA256: doc.SHA256, RelPath: doc.RelPath, SourceName: doc.SourceName})
	}
	return docs
}

// stepSourceDoc is a local, trimmed mirror of step.SourceDoc's fields used in
// these assertions, just to keep the test table terse.
type stepSourceDoc struct {
	Name       string
	SHA256     string
	RelPath    string
	SourceName string
}

// oneSource is the config for a single source "manual" at dir.
func oneSource(dir string) []config.Source {
	return []config.Source{{Name: "manual", Path: dir, Weight: 1.0}}
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

		src, err := NewSourceDocSource(testCtx, oneSource(dir), testLogger)
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

		src, err := NewSourceDocSource(testCtx, oneSource(dir), testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}

		docs := drainSourceDocs(t, &src)
		if len(docs) != 1 || docs[0].Name != "visible.md" {
			t.Fatalf("got docs %+v, want only visible.md", docs)
		}
	})

	t.Run("every yielded doc carries the name of its source and a path relative to the source root", func(t *testing.T) {
		manualDir := t.TempDir()
		changelogDir := t.TempDir()
		writeFile(t, filepath.Join(manualDir, "a.md"), "a")
		writeFile(t, filepath.Join(manualDir, "sub", "b.md"), "b")
		writeFile(t, filepath.Join(changelogDir, "c.md"), "c")

		sources := []config.Source{
			{Name: "manual", Path: manualDir, Weight: 0.8},
			{Name: "changelog", Path: changelogDir, Weight: 0.4},
		}
		src, err := NewSourceDocSource(testCtx, sources, testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}

		docs := drainSourceDocs(t, &src)
		want := map[string]stepSourceDoc{
			"a.md": {RelPath: "a.md", SourceName: "manual"},
			"b.md": {RelPath: filepath.Join("sub", "b.md"), SourceName: "manual"},
			"c.md": {RelPath: "c.md", SourceName: "changelog"},
		}
		if len(docs) != len(want) {
			t.Fatalf("got %d docs, want %d: %+v", len(docs), len(want), docs)
		}
		for _, d := range docs {
			w, ok := want[d.Name]
			if !ok {
				t.Errorf("unexpected doc %q", d.Name)
				continue
			}
			if d.RelPath != w.RelPath || d.SourceName != w.SourceName {
				t.Errorf("doc %q = (rel %q, source %q), want (rel %q, source %q)", d.Name, d.RelPath, d.SourceName, w.RelPath, w.SourceName)
			}
		}
	})

	t.Run("sha256 matches the file's actual content", func(t *testing.T) {
		dir := t.TempDir()
		content := "known content for hashing"
		writeFile(t, filepath.Join(dir, "a.md"), content)

		src, err := NewSourceDocSource(testCtx, oneSource(dir), testLogger)
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

	t.Run("an unreadable file is reported once and skipped, the following file is still yielded", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.md"), "a")
		writeFile(t, filepath.Join(dir, "b.md"), "b")
		writeFile(t, filepath.Join(dir, "c.md"), "c")

		src, err := NewSourceDocSource(testCtx, oneSource(dir), testLogger)
		if err != nil {
			t.Fatalf("NewSourceDocSource() error = %v", err)
		}
		// Paths are collected at construction; removing b.md afterwards makes
		// hashing fail for exactly that entry when it comes up.
		if err := os.Remove(filepath.Join(dir, "b.md")); err != nil {
			t.Fatalf("Remove: %v", err)
		}

		doc, err := src.NextSourceDoc()
		if err != nil || doc.Name != "a.md" {
			t.Fatalf("first NextSourceDoc() = (%q, %v), want (a.md, nil)", doc.Name, err)
		}

		if _, err := src.NextSourceDoc(); err == nil || err == io.EOF {
			t.Fatalf("second NextSourceDoc() error = %v, want a non-EOF error for the unreadable file", err)
		}

		doc, err = src.NextSourceDoc()
		if err != nil || doc.Name != "c.md" {
			t.Fatalf("third NextSourceDoc() = (%q, %v), want (c.md, nil): the unreadable file must not be retried", doc.Name, err)
		}

		if _, err := src.NextSourceDoc(); err != io.EOF {
			t.Fatalf("fourth NextSourceDoc() error = %v, want io.EOF", err)
		}
	})

	t.Run("NextSourceDoc returns io.EOF once exhausted", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "a.md"), "a")

		src, err := NewSourceDocSource(testCtx, oneSource(dir), testLogger)
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
