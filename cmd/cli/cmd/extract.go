package cmd

import (
	"context"
	"fmt"
	"log"
	"rag/internal/extract/step"
	"rag/internal/platform/extraction/extractor"
	"rag/internal/platform/extraction/source"
	"rag/internal/platform/sqlite/extraction"

	"github.com/spf13/cobra"
)

func NewExtractionCmd() *cobra.Command {
	extractCmd := &cobra.Command{
		Use:   "extract",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			inputDirFlag := cmd.Flag("input-dir")
			inputdir := inputDirFlag.Value.String()
			err := createExtractions(inputdir)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	extractCmd.Flags().StringP("input-dir", "i", "build/input", "--input-dir foo/bar or /foo/bar")

	return extractCmd
}

func createExtractions(inputDir string) error {
	ctx := context.Background()
	sourceDocSource, err := source.NewSourceDocSource(inputDir, ctx)
	if err != nil {
		return fmt.Errorf("error creating docs source: %s", err.Error())
	}
	extracedDocSink, err := extraction.NewExtractedDocSink(ctx)
	if err != nil {
		return fmt.Errorf("error creating docs sink: %s", err.Error())
	}
	extractor, err := extractor.NewKreuzbergExtractor()
	if err != nil {
		return fmt.Errorf("error creating docs sink: %s", err.Error())
	}

	err = step.RunExtract(&sourceDocSource, &extracedDocSink, &extractor)
	if err != nil {
		return err
	}

	return nil
}
