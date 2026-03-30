/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"

	kreuzberg "github.com/kreuzberg-dev/kreuzberg/packages/go/v4"
	"github.com/spf13/cobra"
)

// dummyCmd represents the dummy command
var dummyCmd = &cobra.Command{
	Use:   "dummy",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		stuff()
	},
}

func init() {
	rootCmd.AddCommand(dummyCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// dummyCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// dummyCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

func stuff() {
	fileList := []string{"Monitoring.xml", "Joplin.xml"}
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

	for _, fileName := range fileList {
		fileName = "./data/mw-download/" + fileName
		result, err := kreuzberg.ExtractFileWithContext(context.Background(), fileName, config)
		if err != nil {
			fmt.Printf("error processing %s: %s\n", fileName, err.Error())
		} else {
			fmt.Printf("%s\n\n", result.String())
		}
	}

}
