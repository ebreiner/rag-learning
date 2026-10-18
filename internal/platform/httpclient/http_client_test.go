package httpclient

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// the header transport is after the dump in the chain, so the dump never sees the header values
func TestNewDumpDoesNotContainHeaders(t *testing.T) {
	var gotKey, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer server.Close()

	dumpDir := t.TempDir()
	client, err := NewDump(time.Minute, dumpDir, http.Header{"X-Api-Key": {"secret-key"}}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL, strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if gotKey != "secret-key" || gotBody != "payload" {
		t.Errorf("server got key %q, body %q", gotKey, gotBody)
	}

	reqDumps, err := filepath.Glob(filepath.Join(dumpDir, "*-req-dump"))
	if err != nil || len(reqDumps) != 1 {
		t.Fatalf("want 1 req dump, got %v (err %v)", reqDumps, err)
	}
	dump, err := os.ReadFile(reqDumps[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dump), "secret") {
		t.Errorf("dump contains a secret:\n%s", dump)
	}
	if !strings.Contains(string(dump), "payload") {
		t.Errorf("dump must contain the body:\n%s", dump)
	}
}
