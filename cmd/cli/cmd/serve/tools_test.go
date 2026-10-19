package serve

import (
	"context"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mark3labs/mcp-go/mcp"

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
			args: map[string]any{"sources": []any{"manual"}, "metadata": map[string]any{"enerchart_core": false}},
			want: step.Filter{Sources: []string{"manual"}, Metadata: map[string]any{"enerchart_core": false}},
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
		{Name: "manual", Documents: 1, Keys: []step.CatalogKey{{Key: "enerchart_core", Types: []string{"boolean"}, Documents: 1, DistinctValues: 1}}},
	}}
	tool, err := newCatalogTool(catalog, []string{"manual", "changelogs"})
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
		want := `{"name":"manual","documents":1,"keys":[{"key":"enerchart_core","types":["boolean"],"documents":1,"distinct_values":1}]}`
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
