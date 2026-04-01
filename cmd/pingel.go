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
	"rag/internal/db/normalize"
	"strconv"
	"strings"

	kreuzberg "github.com/kreuzberg-dev/kreuzberg/packages/go/v4"
	"github.com/spf13/cobra"
)

// pingelCmd represents the pingel command
var pingelCmd = &cobra.Command{
	Use:   "pingel",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		processDoc()
	},
}

func init() {
	rootCmd.AddCommand(pingelCmd)
}

func processDoc() {
	config := kreuzberg.NewExtractionConfig(
		kreuzberg.WithUseCache(true),
		kreuzberg.WithEnableQualityProcessing(true),
		kreuzberg.WithPdfOptions(
			kreuzberg.WithPdfExtractMetadata(true),
		),
		kreuzberg.WithOCR(
			kreuzberg.WithOCRBackend("tesseract"),
			kreuzberg.WithOCRLanguage("deu"),
		),
		kreuzberg.WithPages(
			kreuzberg.WithExtractPages(true),
		),
		kreuzberg.WithLanguageDetection(
			kreuzberg.WithLanguageDetectionEnabled(true),
			kreuzberg.WithDetectMultiple(true),
		),
	)
	result, err := kreuzberg.ExtractFileWithContext(context.Background(), "./data/hm_I_skript.pdf", config)
	if err != nil {
		log.Fatalf("error extraction failed: %s", err.Error())
	}
	pages := result.Pages
	fmt.Printf("dump all pages: %+v\n")
	if len(pages) == 0 {
		log.Fatal("error no pages found")
	}

	var jsonl string
	for _, page := range pages {

		validHeadings := map[string]struct{}{
			"h1": {},
			"h2": {},
			"h3": {},
			"h4": {},
			"h5": {},
			"h6": {},
		}
		type Block struct {
			Section string `json:"section_name"`
			Level   uint8  `json:"level"`
			Content string `json:"content"`
		}

		fmt.Printf("page dump: %+v\n\n############################################################################\n\n\n\n", page)

		level := 1
		section := ""
		for bk, hBlock := range page.Hierarchy.Blocks {
			fmt.Printf("processing block # %d\n", bk)
			if _, present := validHeadings[hBlock.Level]; present {
				levelString := strings.ReplaceAll(hBlock.Level, "h", "")
				level, _ = strconv.Atoi(levelString)
				section = hBlock.Text
				continue
			} else if hBlock.Level == "body" {
				block := Block{
					Section: section,
					Level:   uint8(level),
					Content: hBlock.Text,
				}
				byteBlock, _ := json.Marshal(block)
				jsonl = jsonl + string(byteBlock)
			} else {
				log.Fatalf("fatal error: unknown level type: %s", hBlock.Level)
			}
		}
	}

	os.WriteFile("./data/hm_I_skript.jsonl", []byte(jsonl), 0640)
}

func writeResult(chunks []kreuzberg.Chunk) {
	var jsonl string
	for index, chunk := range chunks {
		chunk.Content = normalize.NormalizeText(chunk.Content)
		bytes, err := json.Marshal(chunk)
		if err != nil {
			log.Fatalf("error marshalling chunk to json: %s", err.Error())
		}
		if index == 0 {
			jsonl = fmt.Sprintf("%s", string(bytes))
		} else {
			jsonl = fmt.Sprintf("%s\n%s", jsonl, string(bytes))
		}
	}

	if err := os.WriteFile("./data/hm_I_skript.jsonl", []byte(jsonl), 0640); err != nil {
		log.Fatalf("error writing jsonl to file: %s", err.Error())
	}

}
