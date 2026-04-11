package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"log"
	"os"
	"strings"
	"sync"

	search "rag/internal/platform/sqlite"

	kreuzberg "github.com/kreuzberg-dev/kreuzberg/packages/go/v4"
	_ "github.com/mattn/go-sqlite3"
)

var loadDBCmd = &cobra.Command{
	Use:   "load-db",
	Short: "Load docs into sqlite",
	Long: `Loads all documents with embeddings found in the data dir
and inserts the docs, chunks and embeddings into a sqlite inside the data
dir.`,
	Run: func(cmd *cobra.Command, args []string) {
		loadDB("balanced", 768)
	},
}

func init() {
	rootCmd.AddCommand(loadDBCmd)
}

func loadDB(model string, dimensions uint16) {
	ctx, cancel := context.WithCancel(context.Background())
	inputDirPath := fmt.Sprintf("./data/mw-embedded/%s_%d/", model, dimensions)
	inputDir, err := os.ReadDir(inputDirPath)
	if err != nil {
		log.Fatalf("error reading directory: %s", err.Error())
	}

	var errList []search.InsertError
	errChan := make(chan search.InsertError)
	go func() {
		for err := range errChan {
			fmt.Printf("error processing %s: %s\n", err.DocTitle, err.Err)
			errList = append(errList, err)
		}
	}()

	writeChan := make(chan search.ChunkedDoc)
	db, err := search.NewConn()
	if err != nil {
		cancel()
		log.Fatalf("error opening db: %s", err.Error())
	}
	var writerInitWG sync.WaitGroup
	writerInitWG.Add(1)
	var writerDoneWG sync.WaitGroup
	// Add one for the the writer routine to signal when done
	writerDoneWG.Add(1)
	// Add one per doc to process
	writerDoneWG.Add(len(inputDir))
	// Start the writer routine
	go search.HandleChunks(ctx, db, &writerInitWG, &writerDoneWG, writeChan, errChan)
	// Wait until setup is finished
	writerInitWG.Wait()

	var senderWG sync.WaitGroup
	senderWG.Add(len(inputDir))
	for count, fileEntry := range inputDir {
		go func(fileCount int, entry os.DirEntry) {
			filePath := inputDirPath + entry.Name()
			fmt.Printf("processing file # %d %s\n", fileCount, filePath)
			content, err := os.ReadFile(filePath)
			if err != nil {
				errChan <- search.InsertError{
					DocTitle: filePath,
					Err:      err.Error(),
				}
				senderWG.Done()
				return
			}

			doc := search.ChunkedDoc{
				Title: entry.Name(),
			}

			rawChunkList := strings.Split(string(content), "\n")
			for chunkID, rawChunk := range rawChunkList {
				if rawChunk == "" {
					continue
				}
				var chunk kreuzberg.Chunk
				if err := json.Unmarshal([]byte(rawChunk), &chunk); err != nil {
					errChan <- search.InsertError{
						Err:      fmt.Sprintf("error unmarshalling line %d into chunk: %s", chunkID, err.Error()),
						DocTitle: filePath,
						ChunkID:  int64(chunkID),
					}
					fmt.Printf("line: %s\n\n", rawChunk)
					continue
				}
				doc.Chunks = append(doc.Chunks, chunk)
			}
			writeChan <- doc
			senderWG.Done()
		}(count, fileEntry)
	}
	// Await until sender is done sending
	senderWG.Wait()
	// Close the chanel for sending new inserts
	close(writeChan)
	// Wait until the writer routine is done processing the buffered inserts and committing the tx
	writerDoneWG.Wait()
	close(errChan)
	fmt.Printf("process errors: \n")
	for _, err := range errList {
		fmt.Printf("file: %s, chunk id: %d, error: %s\n", err.DocTitle, err.ChunkID, err.Err)
	}
}
