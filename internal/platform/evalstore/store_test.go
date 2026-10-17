package evalstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"rag/internal/eval/step"

	"github.com/google/go-cmp/cmp"
)

func TestLoadGolden(t *testing.T) {
	dir := t.TempDir()

	t.Run("valid file", func(t *testing.T) {
		path := filepath.Join(dir, "golden.json")
		content := `{"_comment": "ignored", "queries": [{"id": "g1", "query": "q", "expect": [{"doc": "a.md", "text_contains": "x"}]}]}`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}

		gs, sum, err := LoadGolden(path)
		if err != nil {
			t.Fatalf("LoadGolden() error = %v", err)
		}
		want := step.GoldenSet{Queries: []step.GoldenQuery{
			{ID: "g1", Query: "q", Expect: []step.Expectation{{Doc: "a.md", TextContains: "x"}}},
		}}
		if diff := cmp.Diff(want, gs); diff != "" {
			t.Errorf("LoadGolden() (-want +got):\n%s", diff)
		}
		if len(sum) != 64 {
			t.Errorf("sha256 = %q, want 64 hex chars", sum)
		}
	})

	t.Run("no queries", func(t *testing.T) {
		path := filepath.Join(dir, "empty.json")
		if err := os.WriteFile(path, []byte(`{"queries": []}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := LoadGolden(path); err == nil {
			t.Error("LoadGolden() error = nil, want error")
		}
	})
}

func TestSaveAndLatest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs") // does not exist yet

	_, _, found, err := LoadLatest(dir)
	if err != nil || found {
		t.Fatalf("LoadLatest() on missing dir = found %v, err %v; want false, nil", found, err)
	}

	older := step.Run{Header: step.RunHeader{Timestamp: time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC), K: 10}}
	newer := step.Run{Header: step.RunHeader{Timestamp: time.Date(2026, 10, 17, 10, 0, 0, 0, time.UTC), K: 5}}

	if _, err := Save(dir, newer); err != nil {
		t.Fatalf("Save(newer) error = %v", err)
	}
	if _, err := Save(dir, older); err != nil {
		t.Fatalf("Save(older) error = %v", err)
	}

	got, path, found, err := LoadLatest(dir)
	if err != nil || !found {
		t.Fatalf("LoadLatest() = found %v, err %v; want true, nil", found, err)
	}
	if filepath.Base(path) != "20261017T100000Z.json" {
		t.Errorf("LoadLatest() path = %s, want 20261017T100000Z.json", path)
	}
	if got.Header.K != 5 {
		t.Errorf("LoadLatest() K = %d, want 5", got.Header.K)
	}

	if _, err := Save(dir, newer); err == nil {
		t.Error("Save() of an existing timestamp: error = nil, want error")
	}
}
