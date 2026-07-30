package cmd

import (
	"context"
	"fmt"
	"log"
	"rag/internal/extract/step"
	"rag/internal/platform/config"
	"rag/internal/platform/extraction/extractor"
	"rag/internal/platform/extraction/source"
	"rag/internal/platform/sqlite"
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
			globals := config.GlobalOptions
			xbergBaseURL, err := config.ResolveGlobal(cmd, globals.XBergURL)
			if err != nil {
				log.Fatal(err)
			}

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				log.Fatal(err)
			}

			inputDirFlag := cmd.Flags().Lookup("input-dir")
			inputDir := inputDirFlag.Value.String()
			if len(inputDir) == 0 {
				log.Fatalf("input directory must be specified")
			}

			err = createExtractions(inputDir, xbergBaseURL, dbPath)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	flags := extractCmd.Flags()
	flags.StringP("input-dir", "i", "", "-i | --input-dir /path/to/input-dir | ./input-dir")

	return extractCmd
}

func createExtractions(inputDir, xbergBaseURL, dbPath string) error {
	ctx := context.Background()
	sourceDocSource, err := source.NewSourceDocSource(inputDir, ctx)
	if err != nil {
		return fmt.Errorf("error creating docs source: %s", err.Error())
	}
	db, err := sqlite.NewConn(dbPath)
	if err != nil {
		return err
	}
	extracedDocSink, err := extraction.NewExtractedDocSink(db, ctx)
	if err != nil {
		return fmt.Errorf("error creating docs sink: %s", err.Error())
	}
	extractor, err := extractor.NewKreuzbergExtractor(xbergBaseURL)
	if err != nil {
		return fmt.Errorf("error creating docs sink: %s", err.Error())
	}

	err = step.RunExtract(&sourceDocSource, &extracedDocSink, &extractor)
	if err != nil {
		return err
	}

	return nil
}
