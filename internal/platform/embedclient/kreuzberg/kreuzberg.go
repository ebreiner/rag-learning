package kreuzberg

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

type ClientKreuzberg struct {
	BaseURL string
	logger  *slog.Logger
	client  *http.Client
}

func NewKreuzbergClient(xbergBaseURL string, logger *slog.Logger, httpClient *http.Client) (ClientKreuzberg, error) {
	client := ClientKreuzberg{}
	if _, err := url.Parse(xbergBaseURL); err != nil {
		return client, err
	}
	client.BaseURL = xbergBaseURL
	client.logger = logger
	client.client = httpClient

	return client, nil
}

type embedResp struct {
	Embeddings [][]float64 `json:"embeddings"`
	Model      string      `json:"model"`
	Dimension  int32       `json:"dimensions"`
}

func (c ClientKreuzberg) runEmbedding(texts []string, ctx context.Context) (embedResp, error) {
	type embedPayload struct {
		Texts []string `json:"texts"`
	}

	payload := embedPayload{
		Texts: texts,
	}
	bytePayload, err := json.Marshal(payload)
	if err != nil {
		return embedResp{}, fmt.Errorf("error marshaling payload: %s", err.Error())
	}
	embedURL, err := url.JoinPath(c.BaseURL, "/embed")
	if err != nil {
		return embedResp{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, embedURL, bytes.NewBufferString(string(bytePayload)))
	if err != nil {
		return embedResp{}, err
	}
	httpResp, err := c.client.Do(req)
	if err != nil {
		return embedResp{}, fmt.Errorf("error received for embedding request: %s", err.Error())
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return embedResp{}, fmt.Errorf("error reading response body: %s", err.Error())
	}
	err = httpResp.Body.Close()
	if err != nil {
		return embedResp{}, fmt.Errorf("error closing response body: %s", err.Error())
	}
	if httpResp.StatusCode != http.StatusOK {
		return embedResp{}, fmt.Errorf("error status code of embedding not 200: %s\n", string(body))
	}

	resp := embedResp{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return embedResp{}, fmt.Errorf("cannot unmarshal embedding response: %s", err.Error())
	} else {
		return resp, nil
	}
}

func (c ClientKreuzberg) EmbedQuery(query string, ctx context.Context) (retrieval.Query, error) {
	resp, err := c.runEmbedding([]string{query}, ctx)
	if err != nil {
		return retrieval.Query{}, err
	}

	if len(resp.Embeddings) == 0 {
		return retrieval.Query{}, fmt.Errorf("response contains no embeddings")
	}

	q := retrieval.Query{
		Vector: resp.Embeddings[0],
		Dim:    int64(resp.Dimension),
		Model:  resp.Model,
	}

	return q, nil
}

func (c ClientKreuzberg) EmbedChunks(chunks []step.ChunkToEmbed, ctx context.Context) (step.EmbeddingsToSave, error) {
	toSave := step.EmbeddingsToSave{}
	texts := make([]string, 0)
	for _, c := range chunks {
		texts = append(texts, c.Text)
	}
	resp, err := c.runEmbedding(texts, ctx)
	if err != nil {
		return toSave, err
	}

	embeddings := []step.Embedding{}
	for i, e := range resp.Embeddings {
		embedding := step.Embedding{
			ChunkID: chunks[i].ChunkID,
			Vector:  e,
		}
		embeddings = append(embeddings, embedding)
	}

	toSave.Dim = int64(resp.Dimension)
	toSave.Model = resp.Model

	return toSave, nil
}
