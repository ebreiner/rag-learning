package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "Main Page", "Main Page"},
		{"umlaut", "Übersicht", "Übersicht"},
		{"slash", "A/B/C", "A_B_C"},
		{"backslash", `A\B`, "A_B"},
		{"windows chars", `a:b*c?d"e<f>g|h`, "a_b_c_d_e_f_g_h"},
		{"control", "a\tb\nc", "a_b_c"},
		{"dot", ".", "_"},
		{"dotdot", "..", "_."},
		{"hidden", ".config", "_config"},
		{"empty", "", "_"},
		{"spaces", "  x  ", "x"},
		{"long", strings.Repeat("a", 300), strings.Repeat("a", 200)},
		{"long multibyte", strings.Repeat("ä", 150), strings.Repeat("ä", 100)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTitle(tt.in); got != tt.want {
				t.Errorf("normalizeTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestOutputDirHelper(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	got, err := OutputDirHelper(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("got %q, want %q", got, dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Errorf("directory not created: %v", err)
	}
}

// TestDownload uses a local httptest server. It checks the continuation, the User-Agent and the written files.
func TestDownload(t *testing.T) {
	const ua = "wikiscrape-test"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.UserAgent() != ua {
			t.Errorf("User-Agent = %q, want %q", r.UserAgent(), ua)
		}
		q := r.URL.Query()
		var resp any
		switch {
		case q.Get("list") == "allpages" && q.Get("apcontinue") == "":
			resp = map[string]any{
				"continue": map[string]string{"apcontinue": "B", "continue": "-||"},
				"query":    map[string]any{"allpages": []map[string]any{{"pageid": 1, "ns": 0, "title": "A/1"}}},
			}
		case q.Get("list") == "allpages" && q.Get("apcontinue") == "B":
			resp = map[string]any{
				"query": map[string]any{"allpages": []map[string]any{{"pageid": 2, "ns": 0, "title": "B"}}},
			}
		case q.Get("prop") == "revisions":
			title := q.Get("titles")
			resp = map[string]any{"query": map[string]any{"pages": []map[string]any{{
				"pageid": 1, "ns": 0, "title": title,
				"revisions": []map[string]any{{"slots": map[string]any{"main": map[string]any{"content": "== " + title + " ==<br>x"}}}},
			}}}}
		default:
			t.Errorf("unexpected request %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL + "/api.php")
	c := newClient(ua, 0)
	pages := c.getArticleList(*u)
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	out := t.TempDir()
	c.getArticles(pages, *u, out)

	got, err := os.ReadFile(filepath.Join(out, "A_1.mw"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "== A/1 ==  x"; string(got) != want {
		t.Errorf("content = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(out, "B.mw")); err != nil {
		t.Error(err)
	}
}

// TestPageMeta checks that the categories of one page merge across a continuation,
// and that the request asks only for the revision timestamp (no user data).
func TestPageMeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("generator") != "allpages" || q.Get("rvprop") != "timestamp" {
			t.Errorf("unexpected request %s", r.URL.RawQuery)
		}
		page := map[string]any{"pageid": 7, "ns": 0, "title": "A/1", "length": 42}
		var resp any
		if q.Get("clcontinue") == "" {
			page["revisions"] = []map[string]any{{"timestamp": "2014-05-06T07:08:09Z"}}
			page["categories"] = []map[string]any{{"ns": 14, "title": "Kategorie:Zeta"}}
			resp = map[string]any{
				"continue": map[string]string{"clcontinue": "7|Alpha", "continue": "||"},
				"query":    map[string]any{"pages": []map[string]any{page}},
			}
		} else {
			page["categories"] = []map[string]any{{"ns": 14, "title": "Category:Alpha"}}
			resp = map[string]any{"query": map[string]any{"pages": []map[string]any{page}}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL + "/api.php")
	metas := newClient("wikiscrape-test", 0).getPageMeta(*u)
	if len(metas) != 1 {
		t.Fatalf("got %d pages, want 1", len(metas))
	}
	m := metas[0]
	if m.File != "A_1" || m.LastEdit != "2014-05-06T07:08:09Z" || m.Length != 42 {
		t.Errorf("got %+v", m)
	}
	if strings.Join(m.Categories, ",") != "Alpha,Zeta" {
		t.Errorf("categories = %v, want [Alpha Zeta]", m.Categories)
	}

	out := t.TempDir()
	writePageMeta(metas, out)
	if _, err := os.Stat(filepath.Join(out, metaFile)); err != nil {
		t.Error(err)
	}
}
