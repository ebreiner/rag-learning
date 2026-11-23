package logging

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestQueryLogTraceIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.log")
	ql, err := NewQueryLogger(WithLogPath(path))
	if err != nil {
		t.Fatalf("NewQueryLogger() error = %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0x23, 0xd9, 0x44, 0x45, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		SpanID:     trace.SpanID{0xa0, 0x1e, 1, 2, 3, 4, 5, 6},
		TraceFlags: trace.FlagsSampled,
	})
	traced := trace.ContextWithSpanContext(context.Background(), sc)

	ql.LogQuery(traced, QueryLog{Query: "q"})
	ql.LogToolCall(traced, ToolCallLog{Tool: "read"})
	ql.LogToolCall(context.Background(), ToolCallLog{Tool: "read"})

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the query log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), raw)
	}
	for i, line := range lines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d is not JSON: %v", i, err)
		}
		if i < 2 {
			if entry["trace_id"] != sc.TraceID().String() || entry["span_id"] != sc.SpanID().String() {
				t.Errorf("line %d: trace_id %v, span_id %v, want %s, %s", i, entry["trace_id"], entry["span_id"], sc.TraceID(), sc.SpanID())
			}
			continue
		}
		if _, ok := entry["trace_id"]; ok {
			t.Errorf("line %d: a context without a span must not write trace_id: %v", i, entry)
		}
	}
}
