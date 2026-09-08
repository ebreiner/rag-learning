package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"rag/internal/embedding/step"
	retrieval "rag/internal/retrieval/step"
)

type ClientOpenAI struct {
	Client  *http.Client
	BaseURL *url.URL
	Model   string
	Dim     int64
	Logger  *slog.Logger
}

func NewOpenAIClient(model, openAIURL string, dim int64, client *http.Client, logger *slog.Logger) (ClientOpenAI, error) {
	openAI := ClientOpenAI{}
	if parsed, err := url.Parse(openAIURL); err == nil {
		openAI.BaseURL = parsed
	} else {
		return openAI, err
	}

	openAI.Client = client
	openAI.Dim = dim
	openAI.Model = model
	openAI.Logger = logger

	return openAI, nil
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

type embedPayload struct {
	Texts []string `json:"input"`
	Model string   `json:"model"`
	Dim   int64    `json:"dimensions"`
}

func (c ClientOpenAI) EmbedQuery(ctx context.Context, query string) (retrieval.Query, error) {
	resp, err := c.runEmbedding(ctx, []string{query})
	if err != nil {
		return retrieval.Query{}, err
	}

	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return retrieval.Query{}, fmt.Errorf("response contains no embeddings")
	}

	if len(resp.Data[0].Embedding) != int(c.Dim) {
		return retrieval.Query{}, fmt.Errorf("mismatch of configured dim and dim of embedded string")
	}

	q := retrieval.Query{
		Vector: resp.Data[0].Embedding,
		Dim:    c.Dim,
		Model:  c.Model,
	}

	return q, nil
}

func (c ClientOpenAI) EmbedChunks(ctx context.Context, chunks []step.ChunkToEmbed) (step.EmbeddingsToSave, error) {
	toSave := step.EmbeddingsToSave{}

	texts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		texts = append(texts, c.Text)
	}

	resp, err := c.runEmbedding(ctx, texts)
	if err != nil {
		return toSave, err
	}
	if len(resp.Data) == 0 {
		return toSave, fmt.Errorf("response contains no embeddings")
	}

	embeddings := []step.Embedding{}

	if len(resp.Data) != len(chunks) {
		return toSave, fmt.Errorf("embedding response mismatch: %d embeddings for %d chunks", len(resp.Data), len(chunks))
	}
	for _, embed := range resp.Data {
		if embed.Index < 0 || embed.Index >= len(chunks) {
			return toSave, fmt.Errorf("embedding index %d out of range for %d chunks", embed.Index, len(chunks))
		}
		if len(embed.Embedding) != int(c.Dim) {
			return toSave, fmt.Errorf("mismatch of configured dim and dim of embedded string")
		}
		embeddings = append(embeddings, step.Embedding{Vector: embed.Embedding, ChunkID: chunks[embed.Index].ChunkID})
	}

	toSave.Dim = c.Dim
	toSave.Model = c.Model
	toSave.Embeddings = embeddings

	return toSave, nil
}

func (c ClientOpenAI) runEmbedding(ctx context.Context, texts []string) (embeddingResponse, error) {
	payload := embedPayload{
		Texts: texts,
		Model: c.Model,
		Dim:   c.Dim,
	}

	bytePayload, err := json.Marshal(payload)
	if err != nil {
		return embeddingResponse{}, fmt.Errorf("error marshaling payload: %w", err)
	}

	embedURL := c.BaseURL.JoinPath("/v1/embeddings")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embedURL.String(), bytes.NewReader(bytePayload))
	if err != nil {
		return embeddingResponse{}, err
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return embeddingResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return embeddingResponse{}, fmt.Errorf("error reading response body: %w", err)
	}
	err = resp.Body.Close()
	if err != nil {
		return embeddingResponse{}, fmt.Errorf("error closing response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return embeddingResponse{}, fmt.Errorf("error status code of embedding not 200: %s", string(body))
	}

	desResp := embeddingResponse{}
	if err := json.Unmarshal(body, &desResp); err != nil {
		return embeddingResponse{}, fmt.Errorf("cannot unmarshal embedding response: %w", err)
	} else {
		return desResp, nil
	}
}
