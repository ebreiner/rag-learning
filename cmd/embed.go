/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"rag/internal/embedding"
	"strings"

	kreuzberg "github.com/kreuzberg-dev/kreuzberg/packages/go/v4"
	"github.com/spf13/cobra"
)

// embedCmd represents the embed command
var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		embed()
	},
}

func init() {
	rootCmd.AddCommand(embedCmd)
}

func embed() {
	inputDir, err := os.ReadDir("./data/mw-chunked/")
	if err != nil {
		log.Fatalf("error reading directory: %s", err.Error())
	}
	type processErr struct {
		Err     string
		File    string
		ChunkID int16
	}

	var errList []processErr
	errChan := make(chan processErr)
	done := make(chan struct{})
	go func() {
		for err := range errChan {
			errList = append(errList, err)
		}
		close(done)
	}()

	for count, fileEntry := range inputDir {
		filePath := "./data/mw-chunked/" + fileEntry.Name()
		fmt.Printf("processing file # %d %s\n", count, filePath)
		content, err := os.ReadFile(filePath)
		if err != nil {
			errChan <- processErr{
				Err:     err.Error(),
				File:    filePath,
				ChunkID: -1,
			}
			continue
		}

		rawChunkList := strings.Split(string(content), "\n")
		var dimensions int32
		var model string
		var processedChunks []kreuzberg.Chunk
		for chunkID, rawChunk := range rawChunkList {
			var chunk kreuzberg.Chunk
			//			fmt.Printf("raw chunk: %s\n", rawChunk)
			if err := json.Unmarshal([]byte(rawChunk), &chunk); err != nil {
				errChan <- processErr{
					Err:     fmt.Sprintf("error unmarshalling line into chunk: %s", err.Error()),
					File:    filePath,
					ChunkID: int16(chunkID),
				}
				continue
			}
			embedding, err := embedding.Embed(context.Background(), chunk.Content)
			if err != nil {
				errChan <- processErr{
					Err:     fmt.Sprintf("error embedding chunk: %s", err.Error()),
					File:    filePath,
					ChunkID: int16(chunkID),
				}
				continue
			} else {
				chunk.Embedding = embedding.Vec
				model = embedding.Model
				dimensions = embedding.Dimensions
			}
			processedChunks = append(processedChunks, chunk)
		}
		split := strings.Split(fileEntry.Name(), ".")
		split = split[:len(split)-1]
		jsonlFileName := strings.Join(split, "") + ".jsonl"
		pwd, _ := os.Getwd()
		modelPart := fmt.Sprintf("%s_%d", model, dimensions)
		jsonlFilePath := filepath.Join(pwd, "data", "mw-embedded", modelPart, jsonlFileName)

		var jsonl string
		for index, chunk := range processedChunks {
			bytes, err := json.Marshal(chunk)
			if err != nil {
				errChan <- processErr{
					Err:     fmt.Sprintf("error marshalling chunk: %s", err.Error()),
					File:    filePath,
					ChunkID: int16(chunk.Metadata.ChunkIndex),
				}
				continue
			}
			if index == 0 {
				jsonl = fmt.Sprintf("%s", string(bytes))
			} else {
				jsonl = fmt.Sprintf("%s\n%s", jsonl, string(bytes))
			}

		}

		if err = os.WriteFile(jsonlFilePath, []byte(jsonl), 0640); err != nil {
			errChan <- processErr{
				Err:     fmt.Sprintf("error writing proccessed file: %s", err.Error()),
				File:    filePath,
				ChunkID: -1,
			}
			continue
		}

	}

	close(errChan)
	fmt.Printf("process errors: \n")
	for _, err := range errList {
		fmt.Printf("file: %s, chunk id: %d, error: %s\n", err.File, err.ChunkID, err.Err)
	}
}
