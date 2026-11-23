package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testCtx = context.Background()

// loadExample loads config.yaml.example after the replacements, as old, new pairs.
func loadExample(t *testing.T, replacements ...string) (Config, error) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "config.yaml.example"))
	if err != nil {
		t.Fatalf("reading example config: %v", err)
	}
	text := string(b)
	for i := 0; i+1 < len(replacements); i += 2 {
		if !strings.Contains(text, replacements[i]) {
			t.Fatalf("example config has no %q", replacements[i])
		}
		text = strings.Replace(text, replacements[i], replacements[i+1], 1)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return Load(testCtx, path)
}

func TestRerankConfig(t *testing.T) {
	t.Run("the example config loads with the rerank off", func(t *testing.T) {
		cnf, err := loadExample(t)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cnf.Retrieval.Rerank.Enabled {
			t.Errorf("rerank is enabled, want off")
		}
	})

	t.Run("a missing rerank block is off and needs no fields", func(t *testing.T) {
		cnf, err := loadExample(t,
			"  rerank:\n    enabled: false\n    url: https://openrouter.ai/api/v1/rerank\n    model: cohere/rerank-4-fast\n    api_key: <your OpenRouter key>\n    timeout: 10s\n", "")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cnf.Retrieval.Rerank.Enabled {
			t.Errorf("rerank is enabled, want off")
		}
	})

	t.Run("enabled with all fields loads, an empty url gives the default", func(t *testing.T) {
		cnf, err := loadExample(t,
			"    enabled: false", "    enabled: true",
			"    url: https://openrouter.ai/api/v1/rerank\n", "")
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		r := cnf.Retrieval.Rerank
		if !r.Enabled || r.Model != "cohere/rerank-4-fast" || r.Timeout != 10*time.Second {
			t.Errorf("rerank = enabled %v, model %q, timeout %v", r.Enabled, r.Model, r.Timeout)
		}
		if r.URL == nil || r.URL.String() != DefaultRerankURL {
			t.Errorf("url = %v, want %s", r.URL, DefaultRerankURL)
		}
	})

	errCases := []struct {
		name         string
		replacements []string
		wantErr      string
	}{
		{"enabled needs a model", []string{"    enabled: false", "    enabled: true", "model: cohere/rerank-4-fast", `model: ""`}, "retrieval.rerank.model: required"},
		{"enabled needs an api key", []string{"    enabled: false", "    enabled: true", "api_key: <your OpenRouter key>", `api_key: ""`}, "retrieval.rerank.api_key: required"},
		{"enabled needs a timeout", []string{"    enabled: false", "    enabled: true", "timeout: 10s\n\nread", "timeout: 0s\n\nread"}, "retrieval.rerank.timeout: must be > 0"},
		{"enabled needs an http url", []string{"    enabled: false", "    enabled: true", "url: https://openrouter.ai", "url: ftp://openrouter.ai"}, "retrieval.rerank.url: scheme"},
	}
	for _, tt := range errCases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := loadExample(t, tt.replacements...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}

	t.Run("disabled does not check the fields", func(t *testing.T) {
		if _, err := loadExample(t, `api_key: <your OpenRouter key>`, `api_key: ""`); err != nil {
			t.Fatalf("Load() error = %v", err)
		}
	})
}
