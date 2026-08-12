package cmd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"rag/internal/extract/step"
	"rag/internal/platform/config"
	"rag/internal/platform/extraction/docling"
	"rag/internal/platform/extraction/source"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/extraction"
	"time"

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
			doclingURL, err := config.ResolveGlobal(cmd, globals.DoclingURL)
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
			inputDir, err = config.ResolvePath(inputDir)
			if err != nil {
				log.Fatal(err)
			}

			dumpDirFlag := cmd.Flags().Lookup("dump-dir")
			dumpDir := dumpDirFlag.Value.String()
			if dumpDirFlag.Changed {
				if len(dumpDir) == 0 {
					log.Fatal("dump-dir cannot be an empty string if specified")
				}
			}

			err = createExtractions(inputDir, doclingURL, dbPath, dumpDir)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	flags := extractCmd.Flags()
	flags.StringP("input-dir", "i", "", "-i | --input-dir /path/to/input-dir | ./input-dir")
	flags.StringP("dump-dir", "d", "", "-d | --dump-dir /where/to/dump/resp-req")

	return extractCmd
}

func createExtractions(inputDir, doclingURL, dbPath, dumpDir string) error {
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

	var client *http.Client
	timeout := time.Second * 1800
	if len(dumpDir) != 0 {
		dumpDir, err := config.ResolvePath(dumpDir)
		if err != nil {
			return err
		}
		client, err = httpclient.NewDump(timeout, dumpDir)
	} else {
		client = httpclient.New(timeout)
	}

	extractor, err := docling.NewDoclingExtractor(doclingURL, client)
	if err != nil {
		return fmt.Errorf("error creating docs sink: %s", err.Error())
	}

	err = step.RunExtract(&sourceDocSource, &extracedDocSink, &extractor)
	if err != nil {
		return err
	}

	return nil
}
