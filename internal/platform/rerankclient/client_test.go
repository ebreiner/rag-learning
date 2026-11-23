package rerankclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"rag/internal/platform/httpclient"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/trace"
)

var testCtx = context.Background()

type fakeRerankServer struct {
	lastRequest rerankPayload
	lastAuth    string
	statusCode  int
	body        string
	delay       time.Duration
}

func (f *fakeRerankServer) handler(w http.ResponseWriter, r *http.Request) {
	f.lastAuth = r.Header.Get("Authorization")
	_ = json.NewDecoder(r.Body).Decode(&f.lastRequest)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	status := f.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write([]byte(f.body))
}

func newTestClient(t *testing.T, server *fakeRerankServer, timeout time.Duration) *Client {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(server.handler))
	t.Cleanup(ts.Close)

	endpoint, err := url.Parse(ts.URL + "/api/v1/rerank")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	header := http.Header{}
	header.Set("Authorization", "Bearer test-key")
	return New(endpoint, "cohere/rerank-4-fast", httpclient.New(timeout, header))
}

func TestRerank(t *testing.T) {
	t.Run("sends model, query, documents and the key, maps the scores to the input order", func(t *testing.T) {
		server := &fakeRerankServer{body: `{"id":"x","model":"cohere/rerank-4-fast","results":[
			{"index":2,"relevance_score":0.9},{"index":0,"relevance_score":0.5},{"index":1,"relevance_score":0.1}],
			"usage":{"cost":0.0012,"search_units":1}}`}
		client := newTestClient(t, server, time.Second)

		got, err := client.Rerank(testCtx, "q", []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("Rerank() error = %v", err)
		}
		if diff := cmp.Diff([]float64{0.5, 0.1, 0.9}, got.Scores); diff != "" {
			t.Errorf("scores mismatch (-want +got):\n%s", diff)
		}
		if got.Model != "cohere/rerank-4-fast" || got.Cost != 0.0012 {
			t.Errorf("model, cost = %q, %g, want cohere/rerank-4-fast, 0.0012", got.Model, got.Cost)
		}
		want := rerankPayload{Model: "cohere/rerank-4-fast", Query: "q", Documents: []string{"a", "b", "c"}}
		if diff := cmp.Diff(want, server.lastRequest); diff != "" {
			t.Errorf("request mismatch (-want +got):\n%s", diff)
		}
		if server.lastAuth != "Bearer test-key" {
			t.Errorf("Authorization header is not the configured key")
		}
	})

	t.Run("sends the trace id and the span id of the caller", func(t *testing.T) {
		server := &fakeRerankServer{body: `{"results":[{"index":0,"relevance_score":1}]}`}
		client := newTestClient(t, server, time.Second)
		sc := trace.NewSpanContext(trace.SpanContextConfig{
			TraceID:    trace.TraceID{1, 2, 3},
			SpanID:     trace.SpanID{4, 5, 6},
			TraceFlags: trace.FlagsSampled,
		})

		got, err := client.Rerank(trace.ContextWithSpanContext(testCtx, sc), "q", []string{"a"})
		if err != nil {
			t.Fatalf("Rerank() error = %v", err)
		}
		want := &rerankTrace{TraceID: sc.TraceID().String(), ParentSpanID: sc.SpanID().String()}
		if diff := cmp.Diff(want, server.lastRequest.Trace); diff != "" {
			t.Errorf("trace mismatch (-want +got):\n%s", diff)
		}
		if got.Model != "cohere/rerank-4-fast" {
			t.Errorf("model = %q, want the configured model when the response has none", got.Model)
		}
	})

	errCases := []struct {
		name    string
		server  *fakeRerankServer
		wantErr string
	}{
		{"status not 200", &fakeRerankServer{statusCode: http.StatusUnauthorized, body: `{"error":"no auth"}`}, "status 401"},
		{"body is not json", &fakeRerankServer{body: `nope`}, "unmarshal"},
		{"fewer results than documents", &fakeRerankServer{body: `{"results":[{"index":0,"relevance_score":1}]}`}, "1 results for 2 documents"},
		{"index out of range", &fakeRerankServer{body: `{"results":[{"index":0,"relevance_score":1},{"index":5,"relevance_score":1}]}`}, "out of range"},
		{"duplicate index", &fakeRerankServer{body: `{"results":[{"index":0,"relevance_score":1},{"index":0,"relevance_score":1}]}`}, "more than once"},
		{"timeout", &fakeRerankServer{delay: 200 * time.Millisecond, body: `{}`}, "rerank request"},
	}
	for _, tt := range errCases {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient(t, tt.server, 50*time.Millisecond)
			_, err := client.Rerank(testCtx, "q", []string{"a", "b"})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Rerank() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}
