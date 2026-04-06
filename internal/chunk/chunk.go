package chunk

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"rag/internal/types"
	"rag/internal/writer"
	"sync"
)

func Chunk(inputDir, outputDir string) error {
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
		go processDoc(doc, errChan, resultChan, &wgProcessing)

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

func processDoc(doc types.Document, errChan chan writer.WriteError, resultChan chan writer.ResultMessage, wgProcessing *sync.WaitGroup) {
	var chunkIndex int
	chunkIndex = 0
	maxSize := 1000
	var chunks []types.Chunk
	var chunk types.Chunk

	// local mini helper to ease up emitting of chunks, tightly coupled to the state this scope!
	emitchunk := func() {
		chunks = append(chunks, chunk)
		chunkIndex++
		chunk = types.Chunk{ChunkIndex: chunkIndex}
	}

	for _, node := range doc.Nodes {

		// code group heading image list list_item paragraph table
		switch node.NodeType {
		case "heading":
			if chunk.Text != "" {
				emitchunk()
			}
			chunk.Text = node.Text

		case "paragraph", "list", "list_item", "code":
			if len(chunk.Text)+len(node.Text) >= maxSize {
				emitchunk()
			}
			chunk.Text += node.Text + "\n"

		default:
			errChan <- writer.WriteError{
				Err: fmt.Sprintf("error processesing node: unkown node type '%s'", node.NodeType),
			}
		}

	}
	if chunk.Text != "" {
		emitchunk()
	}
	for idx, chunk := range chunks {
		cc := chunk
		cc.ChunkCount = chunkIndex
		chunks[idx] = cc
	}
	doc.Chunks = chunks
	docs := make([]types.Document, 1)
	docs[0] = doc
	resultChan <- writer.ResultMessage{
		Documents:     docs,
		FileExtension: "jsonl",
	}
	wgProcessing.Done()
}
