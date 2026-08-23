package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"rag/internal/extract/step"
	"rag/internal/platform/config"
	"rag/internal/platform/extraction/docling"
	"rag/internal/platform/extraction/source"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/extraction"
	"rag/internal/platform/telemetry/logging"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			logger, otelShutdownFunc, err := Setup(ctx, cmd)
			if err != nil {
				return fmt.Errorf("error setting up otel and logger: %w", err)
			}
			logger = logger.With(logging.KeyStep, "extract")
			defer func() {
				if err := otelShutdownFunc(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			globals := config.GlobalOptions
			doclingURL, err := config.ResolveGlobal(cmd, globals.DoclingURL)
			if err != nil {
				return err
			}

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				return err
			}

			inputDirFlag := cmd.Flags().Lookup("input-dir")
			inputDir := inputDirFlag.Value.String()
			if len(inputDir) == 0 {
				return fmt.Errorf("missing required flag input-dir")
			}
			inputDir, err = config.ResolvePath(inputDir)
			if err != nil {
				return err
			}

			dumpDirFlag := cmd.Flags().Lookup("dump-dir")
			dumpDir := dumpDirFlag.Value.String()
			if dumpDirFlag.Changed {
				if len(dumpDir) == 0 {
					return fmt.Errorf("dump-dir cannot be set to empty string if specified")
				}
			}

			err = createExtractions(ctx, inputDir, doclingURL, dbPath, dumpDir, logger)
			if err != nil {
				return err
			}

			return nil
		},
	}

	flags := extractCmd.Flags()
	flags.StringP("input-dir", "i", "", "-i | --input-dir /path/to/input-dir | ./input-dir")
	flags.StringP("dump-dir", "d", "", "-d | --dump-dir /where/to/dump/resp-req")

	return extractCmd
}

func createExtractions(ctx context.Context, inputDir, doclingURL, dbPath, dumpDir string, logger *slog.Logger) error {
	sourceDocSource, err := source.NewSourceDocSource(ctx, inputDir, logger)
	if err != nil {
		return fmt.Errorf("error creating docs source: %w", err)
	}
	db, err := sqlite.NewConn(dbPath, false)
	if err != nil {
		return err
	}

	extracedDocSink, err := extraction.NewExtractedDocSink(db, logger)
	if err != nil {
		return fmt.Errorf("error creating docs sink: %w", err)
	}

	var client *http.Client
	timeout := time.Second * 1800
	if len(dumpDir) != 0 {
		dumpDir, err := config.ResolvePath(dumpDir)
		if err != nil {
			return err
		}
		client, err = httpclient.NewDump(timeout, dumpDir, logger)
		if err != nil {
			return err
		}
	} else {
		client = httpclient.New(timeout)
	}

	extractor, err := docling.NewDoclingExtractor(doclingURL, client, logger)
	if err != nil {
		return fmt.Errorf("error creating docling extractor: %w", err)
	}

	err = step.RunExtract(ctx, &sourceDocSource, &extracedDocSink, &extractor, logger)
	if err != nil {
		return err
	}

	return nil
}
