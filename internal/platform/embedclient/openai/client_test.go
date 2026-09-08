package openai

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"rag/internal/embedding/step"

	"github.com/google/go-cmp/cmp"
)

var (
	testCtx    = context.Background()
	testLogger = slog.New(slog.DiscardHandler)
)

// fakeEmbedServer records the last request it received and replies with a
// canned response built from the requested input's length, so tests can
// control exactly how many embedding vectors come back.
type fakeEmbedServer struct {
	lastRequest embedPayload
	vectorLen   int // dimension of the vectors this fake server hands back
	statusCode  int
	rawBody     string // if set, returned verbatim instead of building a response
}

func (f *fakeEmbedServer) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := json.Marshal(struct{}{})
	_ = body
	var payload embedPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
		f.lastRequest = payload
	}

	status := f.statusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)

	if f.rawBody != "" {
		_, _ = w.Write([]byte(f.rawBody))
		return
	}

	type respEmbedding struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	}
	resp := struct {
		Data []respEmbedding `json:"data"`
	}{}
	for i := range payload.Texts {
		vec := make([]float64, f.vectorLen)
		for j := range vec {
			vec[j] = 0.5
		}
		resp.Data = append(resp.Data, respEmbedding{Embedding: vec, Index: i})
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func newTestClient(t *testing.T, server *fakeEmbedServer, dim int64) (ClientOpenAI, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(server.handler))
	t.Cleanup(ts.Close)

	client, err := NewOpenAIClient("bge-m3", ts.URL, dim, ts.Client(), testLogger)
	if err != nil {
		t.Fatalf("NewOpenAIClient: %v", err)
	}
	return client, ts
}

func TestEmbedChunks(t *testing.T) {
	t.Run("maps response embeddings back onto the right chunk ids in order", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4}
		client, _ := newTestClient(t, server, 4)

		chunks := []step.ChunkToEmbed{
			{ChunkID: 101, Text: "first"},
			{ChunkID: 202, Text: "second"},
		}
		got, err := client.EmbedChunks(testCtx, chunks)
		if err != nil {
			t.Fatalf("EmbedChunks() error = %v", err)
		}

		if got.Model != "bge-m3" || got.Dim != 4 {
			t.Errorf("got Model=%q Dim=%d, want Model=bge-m3 Dim=4", got.Model, got.Dim)
		}
		if len(got.Embeddings) != 2 {
			t.Fatalf("got %d embeddings, want 2", len(got.Embeddings))
		}
		if got.Embeddings[0].ChunkID != 101 || got.Embeddings[1].ChunkID != 202 {
			t.Errorf("chunk ids not aligned: got %d, %d", got.Embeddings[0].ChunkID, got.Embeddings[1].ChunkID)
		}
		if len(got.Embeddings[0].Vector) != 4 {
			t.Errorf("vector length = %d, want 4", len(got.Embeddings[0].Vector))
		}
	})

	t.Run("sends model, dimensions, and input text in the request", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4}
		client, _ := newTestClient(t, server, 4)

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello world"}})
		if err != nil {
			t.Fatalf("EmbedChunks() error = %v", err)
		}

		want := embedPayload{Texts: []string{"hello world"}, Model: "bge-m3", Dim: 4}
		if diff := cmp.Diff(want, server.lastRequest); diff != "" {
			t.Errorf("request payload mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("returned vector length not matching configured dim is a hard error", func(t *testing.T) {
		// server hands back 768-dim vectors, client is configured for 1024 --
		// this is exactly the mismatch class of bug from earlier in this
		// rework (an inverted == vs != check let a real mismatch through).
		server := &fakeEmbedServer{vectorLen: 768}
		client, _ := newTestClient(t, server, 1024)

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello"}})
		if err == nil {
			t.Fatalf("expected an error on dim mismatch, got nil")
		}
	})

	t.Run("matching dim does not error", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 1024}
		client, _ := newTestClient(t, server, 1024)

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello"}})
		if err != nil {
			t.Fatalf("EmbedChunks() unexpected error = %v", err)
		}
	})

	t.Run("non-200 status is a clear error", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4, statusCode: http.StatusInternalServerError, rawBody: "server exploded"}
		client, _ := newTestClient(t, server, 4)

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello"}})
		if err == nil {
			t.Fatalf("expected an error for a 500 response")
		}
		if !strings.Contains(err.Error(), "server exploded") {
			t.Errorf("error %q should include the response body for debuggability", err.Error())
		}
	})

	t.Run("malformed json response is a clear error, not a panic", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4, rawBody: "{not valid json"}
		client, _ := newTestClient(t, server, 4)

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello"}})
		if err == nil {
			t.Fatalf("expected an error for malformed JSON")
		}
	})

	t.Run("empty data array is a clear error", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4, rawBody: `{"data":[]}`}
		client, _ := newTestClient(t, server, 4)

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello"}})
		if err == nil {
			t.Fatalf("expected an error for an empty data array")
		}
	})

	t.Run("fewer embeddings than chunks is a hard error, not a silent misalignment", func(t *testing.T) {
		// Before the length guard, a short response mapped by array position
		// and the trailing chunks simply never got an embedding.
		server := &fakeEmbedServer{rawBody: `{"data":[{"embedding":[0.1,0.2],"index":0}]}`}
		client, _ := newTestClient(t, server, 2)

		chunks := []step.ChunkToEmbed{{ChunkID: 101, Text: "a"}, {ChunkID: 202, Text: "b"}}
		_, err := client.EmbedChunks(testCtx, chunks)
		if err == nil {
			t.Fatalf("expected an error for 1 embedding on 2 chunks, got nil")
		}
	})

	t.Run("more embeddings than chunks is a hard error, not a panic", func(t *testing.T) {
		// Before the length guard this indexed chunks[2] on a 2-element slice.
		server := &fakeEmbedServer{rawBody: `{"data":[` +
			`{"embedding":[0.1,0.2],"index":0},` +
			`{"embedding":[0.3,0.4],"index":1},` +
			`{"embedding":[0.5,0.6],"index":2}]}`}
		client, _ := newTestClient(t, server, 2)

		chunks := []step.ChunkToEmbed{{ChunkID: 101, Text: "a"}, {ChunkID: 202, Text: "b"}}
		_, err := client.EmbedChunks(testCtx, chunks)
		if err == nil {
			t.Fatalf("expected an error for 3 embeddings on 2 chunks, got nil")
		}
	})

	t.Run("embeddings are mapped by their index field, not by array position", func(t *testing.T) {
		// Server returns index 1 first. The vectors are distinguishable so a
		// position-based mapping would visibly swap them.
		server := &fakeEmbedServer{rawBody: `{"data":[` +
			`{"embedding":[1.0,1.0],"index":1},` +
			`{"embedding":[0.0,0.0],"index":0}]}`}
		client, _ := newTestClient(t, server, 2)

		chunks := []step.ChunkToEmbed{{ChunkID: 101, Text: "a"}, {ChunkID: 202, Text: "b"}}
		got, err := client.EmbedChunks(testCtx, chunks)
		if err != nil {
			t.Fatalf("EmbedChunks() error = %v", err)
		}

		byChunk := map[int64][]float64{}
		for _, e := range got.Embeddings {
			byChunk[e.ChunkID] = e.Vector
		}
		want := map[int64][]float64{101: {0.0, 0.0}, 202: {1.0, 1.0}}
		if diff := cmp.Diff(want, byChunk); diff != "" {
			t.Errorf("vector-to-chunk mapping mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("an index outside the request range is a hard error", func(t *testing.T) {
		server := &fakeEmbedServer{rawBody: `{"data":[` +
			`{"embedding":[0.1,0.2],"index":0},` +
			`{"embedding":[0.3,0.4],"index":7}]}`}
		client, _ := newTestClient(t, server, 2)

		chunks := []step.ChunkToEmbed{{ChunkID: 101, Text: "a"}, {ChunkID: 202, Text: "b"}}
		_, err := client.EmbedChunks(testCtx, chunks)
		if err == nil {
			t.Fatalf("expected an error for index 7 on 2 chunks, got nil")
		}
	})

	t.Run("a connection failure is reported as ErrProviderUnreachable", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4}
		client, ts := newTestClient(t, server, 4)
		ts.Close() // nothing listens on the URL any more

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello"}})
		if !errors.Is(err, step.ErrProviderUnreachable) {
			t.Fatalf("error = %v, want one wrapping step.ErrProviderUnreachable", err)
		}
	})

	t.Run("a non-200 response is NOT ErrProviderUnreachable, it stays on the model-failure path", func(t *testing.T) {
		// A 500 from Ollama can be a genuine per-batch model failure, which
		// is exactly what embedWithFallback's bisect is for.
		server := &fakeEmbedServer{vectorLen: 4, statusCode: http.StatusInternalServerError, rawBody: "model choked"}
		client, _ := newTestClient(t, server, 4)

		_, err := client.EmbedChunks(testCtx, []step.ChunkToEmbed{{ChunkID: 1, Text: "hello"}})
		if err == nil {
			t.Fatalf("expected an error for a 500 response")
		}
		if errors.Is(err, step.ErrProviderUnreachable) {
			t.Errorf("a 500 response must not be classified as unreachable: %v", err)
		}
	})
}

func TestEmbedQuery(t *testing.T) {
	t.Run("returns a Query carrying the vector, dim, and model", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4}
		client, _ := newTestClient(t, server, 4)

		q, err := client.EmbedQuery(testCtx, "what is single sign on")
		if err != nil {
			t.Fatalf("EmbedQuery() error = %v", err)
		}
		if q.Model != "bge-m3" || q.Dim != 4 {
			t.Errorf("got Model=%q Dim=%d, want Model=bge-m3 Dim=4", q.Model, q.Dim)
		}
		if len(q.Vector) != 4 {
			t.Errorf("vector length = %d, want 4", len(q.Vector))
		}
	})

	t.Run("dim mismatch is a hard error here too", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 768}
		client, _ := newTestClient(t, server, 1024)

		_, err := client.EmbedQuery(testCtx, "hello")
		if err == nil {
			t.Fatalf("expected an error on dim mismatch")
		}
	})

	t.Run("empty data array is a clear error, not a panic", func(t *testing.T) {
		server := &fakeEmbedServer{vectorLen: 4, rawBody: `{"data":[]}`}
		client, _ := newTestClient(t, server, 4)

		_, err := client.EmbedQuery(testCtx, "hello")
		if err == nil {
			t.Fatalf("expected an error for an empty data array")
		}
	})
}
