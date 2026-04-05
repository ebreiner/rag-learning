package writer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"rag/internal/types"
	"sync"
)

type WriteError struct {
	OutputPath string
	Err        string
}

func (e WriteError) Error() string {
	return fmt.Sprintf("error writing file %s: %s", e.OutputPath, e.Err)
}

type ResultMessage struct {
	Documents     []types.Document
	FileExtension string
}

func HandleWrites(outputDir string, wgDone *sync.WaitGroup, errChan chan WriteError, resultChan chan ResultMessage) {
	fmt.Println("writer started and ready..")
	for resultMessage := range resultChan {
		for _, document := range resultMessage.Documents {
			name := document.Title
			outputFileName := name + "." + resultMessage.FileExtension
			outputPath := filepath.Join(outputDir, outputFileName)
			content, err := json.Marshal(document)
			if err != nil {
				errChan <- WriteError{
					OutputPath: outputPath,
					Err:        fmt.Sprintf("cannot marshal json in writer: %s", err.Error()),
				}
				continue
			}

			if err := os.WriteFile(outputPath, content, 0640); err != nil {
				errChan <- WriteError{
					OutputPath: outputPath,
					Err:        fmt.Sprintf("cannot write file %s: %s", outputPath, err.Error()),
				}
				continue
			}
		}
	}
	fmt.Println("writer done, shutting down..")
	wgDone.Done()
	close(errChan)
}
