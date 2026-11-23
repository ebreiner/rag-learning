package rerankclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"rag/internal/retrieval/step"

	"go.opentelemetry.io/otel/trace"
)

// Client calls a rerank endpoint in the OpenRouter/Cohere format.
// The http client adds the auth header, so the Client does not keep the key.
type Client struct {
	Client   *http.Client
	Endpoint *url.URL
	Model    string
}

func New(endpoint *url.URL, model string, client *http.Client) *Client {
	return &Client{Client: client, Endpoint: endpoint, Model: model}
}

type rerankTrace struct {
	TraceID      string `json:"trace_id"`
	ParentSpanID string `json:"parent_span_id"`
}

type rerankPayload struct {
	Model     string       `json:"model"`
	Query     string       `json:"query"`
	Documents []string     `json:"documents"`
	Trace     *rerankTrace `json:"trace,omitempty"`
}

type rerankResponse struct {
	Model   string `json:"model"`
	Results []struct {
		Index          int     `json:"index"`
		RelevanceScore float64 `json:"relevance_score"`
	} `json:"results"`
	Usage struct {
		Cost float64 `json:"cost"`
	} `json:"usage"`
}

func (c *Client) Rerank(ctx context.Context, query string, texts []string) (step.RerankResult, error) {
	payload := rerankPayload{Model: c.Model, Query: query, Documents: texts}
	// OpenRouter Broadcast puts the call into the trace of the caller
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		payload.Trace = &rerankTrace{TraceID: sc.TraceID().String(), ParentSpanID: sc.SpanID().String()}
	}

	bytePayload, err := json.Marshal(payload)
	if err != nil {
		return step.RerankResult{}, fmt.Errorf("error marshaling payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint.String(), bytes.NewReader(bytePayload))
	if err != nil {
		return step.RerankResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Client.Do(req)
	if err != nil {
		return step.RerankResult{}, fmt.Errorf("rerank request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return step.RerankResult{}, fmt.Errorf("error reading response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return step.RerankResult{}, fmt.Errorf("rerank status %d: %s", resp.StatusCode, string(body))
	}

	var desResp rerankResponse
	if err := json.Unmarshal(body, &desResp); err != nil {
		return step.RerankResult{}, fmt.Errorf("cannot unmarshal rerank response: %w", err)
	}
	if len(desResp.Results) != len(texts) {
		return step.RerankResult{}, fmt.Errorf("rerank response has %d results for %d documents", len(desResp.Results), len(texts))
	}

	scores := make([]float64, len(texts))
	seen := make([]bool, len(texts))
	for _, r := range desResp.Results {
		if r.Index < 0 || r.Index >= len(texts) {
			return step.RerankResult{}, fmt.Errorf("rerank index %d out of range for %d documents", r.Index, len(texts))
		}
		if seen[r.Index] {
			return step.RerankResult{}, fmt.Errorf("rerank index %d occurs more than once", r.Index)
		}
		seen[r.Index] = true
		scores[r.Index] = r.RelevanceScore
	}

	model := desResp.Model
	if model == "" {
		model = c.Model
	}
	return step.RerankResult{Scores: scores, Model: model, Cost: desResp.Usage.Cost}, nil
}
