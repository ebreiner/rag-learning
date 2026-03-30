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
	"rag/internal/db/normalize"
	"strings"
	"sync"
	"time"

	"github.com/kreuzberg-dev/kreuzberg/packages/go/v4"
	"github.com/spf13/cobra"
)

// chunkCmd represents the chunk command
var chunkCmd = &cobra.Command{
	Use:   "chunk",
	Short: "Chunks current data dir",
	Long: `Chunks all files inside the data dir based on file endings (for now)
	for all types supported:
	  - xml
	  - thats it`,
	Run: func(cmd *cobra.Command, args []string) {
		chunk()
	},
}

func init() {
	rootCmd.AddCommand(chunkCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// chunkCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// chunkCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// TODO:
// normalize names
// mapping / lookup table je nachdem was kreuzberg kann, bsp .{xml,xslt,xsd} -> xml, md,markdown -> markdown (ermöglicht auch spezielle fälle wie README oder so abzubilden)
// ansonsten nur verarbeiten wenn absolut sicher und früh scheitern, alles verhindern was schmutz bzw noise verursacht
func chunk() {
	lang := "eng+deu"
	chunkSize := 500
	chunkOverlap := 50
	useCache := true
	enableQuality := true
	detectMultiple := true

	config := &kreuzberg.ExtractionConfig{
		OCR: &kreuzberg.OCRConfig{
			Backend:  "tesseract",
			Language: &lang,
		},
		Chunking: &kreuzberg.ChunkingConfig{
			ChunkSize:    &chunkSize,
			ChunkOverlap: &chunkOverlap,
		},
		LanguageDetection: &kreuzberg.LanguageDetectionConfig{
			Enabled:        &useCache,
			DetectMultiple: &detectMultiple,
		},
		UseCache:                &useCache,
		EnableQualityProcessing: &enableQuality,
	}

	inputDir, err := os.ReadDir("./data/mw-converted/")
	if err != nil {
		log.Fatalf("error reading directory: %s", err.Error())
	}

	type processErr struct {
		FileName  string
		ErrString string
		Processor string // TODO: iota
	}
	errChan := make(chan processErr)
	var processErrList []processErr
	done := make(chan struct{})
	go func() {
		for err := range errChan {
			processErrList = append(processErrList, err)
		}
		close(done)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(len(inputDir))
	for fileCount, inputFileEntry := range inputDir {
		go func(count int, fileName string) {
			defer wg.Done()
			filePath := "./data/mw-converted/" + fileName
			if inputFileEntry.IsDir() {
				fmt.Printf("directory found (%s), skipping..\n", filePath)
			} else {
				fmt.Printf("processing file # %d: %s\n", fileCount, filePath)
				fileEndings := strings.Split(fileName, ".")
				fileEnding := fileEndings[len(fileEndings)-1]
				fileEnding = strings.ToLower(fileEnding)
				err := processFile(filePath, ctx, config)
				if err != nil {
					errChan <- processErr{
						FileName:  filePath,
						ErrString: err.Error(),
						Processor: fileEnding,
					}
				}
			}
			fmt.Printf("finished processing file # %d: %s\n", count, filePath)
		}(fileCount, inputFileEntry.Name())
	}

	wg.Wait()
	close(errChan)
	<-done
	fmt.Print("finished processing\n\nlist of files with processing errors: \n")
	for _, processErr := range processErrList {
		fmt.Printf("file: %s, type: %s, error: %s\n", processErr.FileName, processErr.Processor, processErr.ErrString)
	}
}

func processFile(filePath string, ctx context.Context, extractionConfig *kreuzberg.ExtractionConfig) error {
	result, err := kreuzberg.ExtractFileWithContext(ctx, filePath, extractionConfig)
	if err != nil {
		return err
	}
	var jsonl string
	for index, chunk := range result.Chunks {
		chunk.Content = normalize.NormalizeText(chunk.Content)
		bytes, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		if index == 0 {
			jsonl = fmt.Sprintf("%s", string(bytes))
		} else {
			jsonl = fmt.Sprintf("%s\n%s", jsonl, string(bytes))
		}
	}
	fmt.Printf("%s\n", filePath)
	pathSplit := strings.Split(filePath, "/")
	pathSegment := pathSplit[len(pathSplit)-1]

	fileSplit := strings.Split(pathSegment, ".")
	filename := strings.Join(fileSplit[:len(fileSplit)-1], "") + ".jsonl"
	fmt.Printf("file name: %s\n", filename)
	pwd, _ := os.Getwd()
	jsonlFilePath := filepath.Join(pwd, "data", "mw-chunked", filename)

	if err := os.WriteFile(jsonlFilePath, []byte(jsonl), 0640); err != nil {
		return err
	}

	fmt.Printf("summary for %s: %s\n", filePath, result.String())
	return nil
}
