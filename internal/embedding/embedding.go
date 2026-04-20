package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"rag/internal/ingest"
	"rag/internal/writer"
	"sync"
)

type embedding struct {
	Text      string
	Embedding []float64
}

// TODO: implement context, move wg out of processing function embedDoc lifecycle should be handled from outside
func EmbedInputDir(inputDir, outputDir string) error {
	inputEntryList, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("error reading input dir %s: %s", inputDir, err.Error())
	}

	var wgWriter sync.WaitGroup
	wgWriter.Add(1)

	errChan := make(chan writer.WriteError)
	var processErrs []writer.WriteError

	resultChan := make(chan writer.ResultMessage)

	go writer.HandleWrites(outputDir, &wgWriter, errChan, resultChan)

	var wgProcessing sync.WaitGroup
	wgProcessing.Add(len(inputEntryList))

	sem := make(chan struct{}, 10)
	for index, entry := range inputEntryList {
		// Aquire slot
		sem <- struct{}{}

		go func() {
			defer wgProcessing.Done()
			defer func() { <-sem }() // release slot

			fmt.Printf("processing file # %d: %s\n", index, entry.Name())

			inputPath := filepath.Join(inputDir, entry.Name())
			content, err := os.ReadFile(inputPath)
			if err != nil {
				errChan <- writer.WriteError{OutputPath: entry.Name(), Err: fmt.Sprintf("error reading file %s: %s", entry.Name(), err.Error())}
				return
			}
			var doc ingest.Document
			if err := json.Unmarshal(content, &doc); err != nil {
				errChan <- writer.WriteError{OutputPath: entry.Name(), Err: fmt.Sprintf("cannot unmarshal json from %s: %s", entry.Name(), err.Error())}
				return
			}

			embedDoc(doc, resultChan, errChan)
		}()
	}

	wgProcessing.Wait()
	close(resultChan)
	wgWriter.Wait()

	//	close(errChan)
	for err := range errChan {
		processErrs = append(processErrs, err)
	}

	if len(processErrs) > 0 {
		var errString string
		for count, err := range processErrs {
			errString = errString + fmt.Sprintf("# %d %s: %s\n", count, err.OutputPath, err.Err)
		}
		return fmt.Errorf(errString)
	}

	return nil
}

func EmbedQuery(query string) ([][]float64, error) {
	emb := embedding{Text: query}
	var embList []embedding
	var embeddings [][]float64
	embList[0] = emb
	embList, err := embedStrings(embList)
	for _, emb := range embList {
		embeddings = append(embeddings, emb.Embedding)
	}
	if err != nil {
		return embeddings, err
	}

	return embeddings, nil
}

func embedStrings(inputs []embedding) ([]embedding, error) {
	embeddings := inputs

	type embedPayload struct {
		Texts []string `json:"texts"`
	}
	var texts []string
	for _, emb := range inputs {
		texts = append(texts, emb.Text)
	}
	payload := embedPayload{
		Texts: texts,
	}
	bytePayload, err := json.Marshal(payload)
	if err != nil {
		return embeddings, fmt.Errorf("error marshaling payload: %s", err.Error())
	}

	httpResp, err := http.Post("http://127.0.0.1:8000/embed", "application/json", bytes.NewBufferString(string(bytePayload)))
	if err != nil {
		return embeddings, fmt.Errorf("error received for embedding request: %s", err.Error())
	}

	body, _ := io.ReadAll(httpResp.Body)
	httpResp.Body.Close()
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

	for index := range embeddings {
		embeddings[index].Embedding = embedResp.Embeddings[index]
	}

	return embeddings, nil
}

func embedDoc(doc ingest.Document, resultChan chan writer.ResultMessage, errChan chan writer.WriteError) {
	errHelper := func(errString string) {
		errChan <- writer.WriteError{
			Err: errString,
		}
	}
	var embeddings []embedding
	for _, chunk := range doc.Chunks {
		emb := embedding{
			Text: chunk.Text,
		}
		embeddings = append(embeddings, emb)
	}

	embeddings, err := embedStrings(embeddings)
	if err != nil {
		errHelper(fmt.Sprintf("error embedding texts: %s", err.Error()))
	}

	for index := range embeddings {
		if embeddings[index].Text != doc.Chunks[index].Text {
			errHelper("error text of chunk and embedd are not as programm expects, exiting")
			return
		} else {
			doc.Chunks[index].Embedding = embeddings[index].Embedding
		}
	}

	var docs []ingest.Document
	docs = append(docs, doc)

	resultChan <- writer.ResultMessage{Documents: docs, FileExtension: "jsonl"}
}
