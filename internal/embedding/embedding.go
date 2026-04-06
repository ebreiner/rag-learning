package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"rag/internal/types"
	"rag/internal/writer"
	"sync"
)

type embedding struct {
	Text      string
	Embedding []float64
}

func EmbedInputDir(inputDir, outputDir string) error {
	inputEntryList, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("error reading input dir %s: %s", inputDir, err.Error())
	}

	var wgWriter sync.WaitGroup
	wgWriter.Add(1)

	var wgErrDone sync.WaitGroup
	wgErrDone.Add(1)

	errChan := make(chan writer.WriteError)
	var processErrs []writer.WriteError
	go func() {
		for err := range errChan {
			processErrs = append(processErrs, err)
		}
		wgErrDone.Done()
	}()

	resultChan := make(chan writer.ResultMessage)

	go writer.HandleWrites(outputDir, &wgWriter, errChan, resultChan)

	var wgProcessing sync.WaitGroup
	wgProcessing.Add(len(inputEntryList))

	for index, entry := range inputEntryList {
		fmt.Printf("processing file # %d: %s\n", index, entry.Name())
		inputPath := filepath.Join(inputDir, entry.Name())
		content, err := os.ReadFile(inputPath)
		if err != nil {
			return fmt.Errorf("error reading file %s: %s", entry.Name(), err.Error())
		}
		var doc types.Document
		if err := json.Unmarshal(content, &doc); err != nil {
			return fmt.Errorf("cannot unmarshal json from %s: %s", entry.Name(), err.Error())
		}

		go embedDoc(doc, resultChan, errChan, &wgProcessing)

	}
	wgProcessing.Wait()
	close(resultChan)
	wgWriter.Wait()
	wgErrDone.Wait()

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
	fmt.Printf("embeding dump: %+v\n", embeddings)

	return embeddings, nil
}

func embedDoc(doc types.Document, resultChan chan writer.ResultMessage, errChan chan writer.WriteError, wg *sync.WaitGroup) {
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

	var docs []types.Document
	docs = append(docs, doc)

	resultChan <- writer.ResultMessage{Documents: docs, FileExtension: "jsonl"}
	wg.Done()
}
