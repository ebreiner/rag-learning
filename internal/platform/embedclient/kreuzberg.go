package embedclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type ClientKreuzberg struct {
	BaseURL string
}

func NewKreuzbergClient(xbergBaseURL string) (ClientKreuzberg, error) {
	client := ClientKreuzberg{}
	if _, err := url.Parse(xbergBaseURL); err != nil {
		return client, err
	}
	client.BaseURL = xbergBaseURL

	return client, nil
}

func (c ClientKreuzberg) Embed(texts []string) ([][]float64, error) {
	embeddings := make([][]float64, 0)
	type embedPayload struct {
		Texts []string `json:"texts"`
	}

	payload := embedPayload{
		Texts: texts,
	}
	bytePayload, err := json.Marshal(payload)
	if err != nil {
		return embeddings, fmt.Errorf("error marshaling payload: %s", err.Error())
	}
	httpResp, err := http.Post(c.BaseURL+"/embed", "application/json", bytes.NewBufferString(string(bytePayload)))
	if err != nil {
		return embeddings, fmt.Errorf("error received for embedding request: %s", err.Error())
	}

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return embeddings, fmt.Errorf("error reading response body: %s", err.Error())
	}
	err = httpResp.Body.Close()
	if err != nil {
		return embeddings, fmt.Errorf("error closing response body: %s", err.Error())
	}
	if httpResp.StatusCode != http.StatusOK {
		return embeddings, fmt.Errorf("error status code of embedding not 200: %s\n", string(body))
	}

	var embedResp struct {
		Embeddings [][]float64 `json:"embeddings"`
		Model      string      `json:"model"`
		Dimension  int32       `json:"dimensions"`
	}

	if err := json.Unmarshal(body, &embedResp); err != nil {
		return embeddings, fmt.Errorf("cannot unmarshal embedding response: %s", err.Error())
	}

	return append(embeddings, embedResp.Embeddings...), nil
}
