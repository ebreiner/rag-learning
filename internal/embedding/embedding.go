package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Embedding struct {
	Vec        []float32
	Model      string
	Dimensions int32
}

func Embed(ctx context.Context, input string) (Embedding, error) {
	embedding := Embedding{
		Model:      "balanced",
		Dimensions: 768,
	}
	type embedPayload struct {
		Texts []string `json:"texts"`
	}
	texts := []string{input}
	payload := embedPayload{
		Texts: texts,
	}
	bytePayload, err := json.Marshal(payload)
	if err != nil {
		return embedding, fmt.Errorf("errror marshaling payload: %s", err.Error())
	}
	resp, err := http.Post("http://127.0.0.1:8000/embed", "application/json", bytes.NewBufferString(string(bytePayload)))
	if err != nil {
		return embedding, fmt.Errorf("error received for embedding request: %s", err.Error())
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var embedResp struct {
		Embeddings [][]float32 `json:"embeddings"`
		Model      string      `json:"model"`
		Dimension  int32       `json:"dimensions"`
	}
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return embedding, fmt.Errorf("error unmarshalling embedding response: %s", err.Error())
	}
	embedding.Vec = embedResp.Embeddings[0]
	return embedding, nil
}
