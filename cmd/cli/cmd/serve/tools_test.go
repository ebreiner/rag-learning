package serve

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mark3labs/mcp-go/mcp"

	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"
)

func TestFilterFromArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]any
		want    step.Filter
		wantErr bool
	}{
		{name: "no filter arguments", args: map[string]any{"retrieval_query": "q"}, want: step.Filter{}},
		{name: "null arguments", args: map[string]any{"sources": nil, "metadata": nil}, want: step.Filter{}},
		{name: "empty metadata", args: map[string]any{"metadata": map[string]any{}}, want: step.Filter{}},
		{
			name: "sources and metadata",
			args: map[string]any{"sources": []any{"manual"}, "metadata": map[string]any{"product_core": false}},
			want: step.Filter{Sources: []string{"manual"}, Metadata: map[string]any{"product_core": false}},
		},
		{name: "sources is a string", args: map[string]any{"sources": "manual"}, wantErr: true},
		{name: "sources has a number", args: map[string]any{"sources": []any{"manual", 1.0}}, wantErr: true},
		{name: "metadata is a list", args: map[string]any{"metadata": []any{"x"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := filterFromArgs(tt.args)
			if tt.wantErr != (err != nil) {
				t.Fatalf("filterFromArgs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil {
				if diff := cmp.Diff(tt.want, got); diff != "" {
					t.Errorf("filter (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestCatalogTool(t *testing.T) {
	catalog := step.Catalog{Sources: []step.CatalogSource{
		{Name: "manual", Documents: 1, Keys: []step.CatalogKey{{Key: "product_core", Types: []string{"boolean"}, Documents: 1, DistinctValues: 1}}},
	}}
	tool, err := newCatalogTool(catalog, []string{"manual", "changelogs"}, discardQueryLogger(t))
	if err != nil {
		t.Fatalf("newCatalogTool() error = %v", err)
	}
	call := func(args map[string]any) *mcp.CallToolResult {
		t.Helper()
		request := mcp.CallToolRequest{}
		request.Params.Arguments = args
		result, err := tool.Handler(context.Background(), request)
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
		return result
	}
	text := func(result *mcp.CallToolResult) string {
		t.Helper()
		content, ok := result.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatalf("content = %T, want mcp.TextContent", result.Content[0])
		}
		return content.Text
	}

	t.Run("the description names the sources", func(t *testing.T) {
		if !strings.Contains(tool.Tool.Description, "Sources: manual, changelogs.") {
			t.Errorf("description = %q, want the source list", tool.Tool.Description)
		}
	})

	t.Run("gives the catalog of one source", func(t *testing.T) {
		want := `{"name":"manual","documents":1,"keys":[{"key":"product_core","types":["boolean"],"documents":1,"distinct_values":1}]}`
		if got := text(call(map[string]any{"source": "manual"})); got != want {
			t.Errorf("text = %s, want %s", got, want)
		}
	})

	t.Run("a configured source without documents gives an empty entry", func(t *testing.T) {
		want := `{"name":"changelogs","documents":0,"keys":[]}`
		if got := text(call(map[string]any{"source": "changelogs"})); got != want {
			t.Errorf("text = %s, want %s", got, want)
		}
	})

	t.Run("an unknown or missing source is a tool error", func(t *testing.T) {
		for _, args := range []map[string]any{{"source": "nope"}, {}} {
			if result := call(args); !result.IsError {
				t.Errorf("args %v: IsError = false, want a tool error", args)
			}
		}
	})
}

type fakeLoader struct {
	doc step.LoadedDocument
	err error
}

func (f *fakeLoader) DocumentOfChunk(ctx context.Context, chunkID int64, filter step.Filter) (step.LoadedDocument, error) {
	return f.doc, f.err
}

func TestReadTool(t *testing.T) {
	doc := step.LoadedDocument{Name: "doc.md", Source: "manual", Metadata: map[string]any{}, Chunks: []step.DocChunk{
		{ID: 1, Breadcrumb: "A", Text: "A\n\nfirst"},
		{ID: 2, Breadcrumb: "B", Text: "B\n\nsecond"},
	}}
	call := func(loader *fakeLoader, args map[string]any) *mcp.CallToolResult {
		t.Helper()
		tool := newReadTool(step.ReadDeps{Loader: loader, MaxChars: 30000, Sources: []string{"manual"}}, discardQueryLogger(t), slog.New(slog.DiscardHandler))
		request := mcp.CallToolRequest{}
		request.Params.Arguments = args
		result, err := tool.Handler(context.Background(), request)
		if err != nil {
			t.Fatalf("handler error = %v", err)
		}
		return result
	}

	t.Run("a section call gives the JSON result", func(t *testing.T) {
		result := call(&fakeLoader{doc: doc}, map[string]any{"chunk_id": 2.0, "mode": "section"})
		text := result.Content[0].(mcp.TextContent).Text
		if result.IsError || !strings.Contains(text, `"text":"## B\n\nsecond"`) || !strings.Contains(text, `"mode":"section"`) {
			t.Errorf("result = %s, want the section text", text)
		}
	})

	t.Run("request and lookup errors are tool errors with the reason", func(t *testing.T) {
		for name, tc := range map[string]struct {
			loader *fakeLoader
			args   map[string]any
		}{
			"missing chunk_id":  {&fakeLoader{doc: doc}, map[string]any{"mode": "section"}},
			"unknown mode":      {&fakeLoader{doc: doc}, map[string]any{"chunk_id": 1.0, "mode": "all"}},
			"bad filter type":   {&fakeLoader{doc: doc}, map[string]any{"chunk_id": 1.0, "mode": "section", "sources": "manual"}},
			"not in the filter": {&fakeLoader{err: step.ErrChunkNotInFilter}, map[string]any{"chunk_id": 1.0, "mode": "section"}},
			"unknown chunk":     {&fakeLoader{err: step.ErrChunkNotFound}, map[string]any{"chunk_id": 9.0, "mode": "section"}},
		} {
			if result := call(tc.loader, tc.args); !result.IsError {
				t.Errorf("%s: IsError = false, want a tool error", name)
			}
		}
	})
}

func discardQueryLogger(t *testing.T) *logging.QueryLogger {
	t.Helper()
	ql, err := logging.NewQueryLogger()
	if err != nil {
		t.Fatalf("NewQueryLogger() error = %v", err)
	}
	return ql
}

func TestToolCallLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.log")
	ql, err := logging.NewQueryLogger(logging.WithLogPath(path))
	if err != nil {
		t.Fatalf("NewQueryLogger() error = %v", err)
	}
	doc := step.LoadedDocument{Name: "doc.md", Source: "manual", Chunks: []step.DocChunk{{ID: 1, Breadcrumb: "A", Text: "A\n\nfirst"}}}
	tool := newReadTool(step.ReadDeps{Loader: &fakeLoader{doc: doc}, MaxChars: 30000, Sources: []string{"manual"}}, ql, slog.New(slog.DiscardHandler))

	for _, args := range []map[string]any{
		{"chunk_id": 1.0, "mode": "section"},
		{"chunk_id": 1.0, "mode": "all"},
	} {
		request := mcp.CallToolRequest{}
		request.Params.Arguments = args
		if _, err := tool.Handler(context.Background(), request); err != nil {
			t.Fatalf("handler error = %v", err)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the query log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2:\n%s", len(lines), raw)
	}
	var got []map[string]any
	for _, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %v", err)
		}
		got = append(got, entry)
	}
	if got[0]["msg"] != "tool_call" || got[0]["tool"] != "read" || got[0]["result"] != "text" || got[0]["chars"].(float64) <= 0 {
		t.Errorf("first entry = %v, want a read call with result text and chars", got[0])
	}
	if args, _ := got[0]["args"].(map[string]any); args["mode"] != "section" {
		t.Errorf("first entry args = %v, want mode section", got[0]["args"])
	}
	if got[1]["result"] != "error" {
		t.Errorf("second entry result = %v, want error", got[1]["result"])
	}
	if _, ok := got[0]["session"]; !ok {
		t.Errorf("first entry has no session field: %v", got[0])
	}
}
