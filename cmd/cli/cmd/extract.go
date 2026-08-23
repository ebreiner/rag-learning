package cmd

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"rag/internal/extract/step"
	"rag/internal/platform/config"
	"rag/internal/platform/extraction/docling"
	"rag/internal/platform/extraction/source"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/extraction"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/platform/telemetry/tracing"
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
			ctx := context.Background()
			logger, err := logging.FromCommand(cmd)
			if err != nil {
				log.Fatal(fmt.Errorf("error setting up logger: %w", err))
			}
			logger = logger.With(logging.KeyStep, "extract")

			shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}
			defer func() {
				if err := shutdownOTEL(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
					os.Exit(1)
				}
			}()

			globals := config.GlobalOptions
			doclingURL, err := config.ResolveGlobal(cmd, globals.DoclingURL)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			inputDirFlag := cmd.Flags().Lookup("input-dir")
			inputDir := inputDirFlag.Value.String()
			if len(inputDir) == 0 {
				logger.ErrorContext(ctx, "wiring", "err", "missing required flag input-dir")
				os.Exit(1)
			}
			inputDir, err = config.ResolvePath(inputDir)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			dumpDirFlag := cmd.Flags().Lookup("dump-dir")
			dumpDir := dumpDirFlag.Value.String()
			if dumpDirFlag.Changed {
				if len(dumpDir) == 0 {
					logger.ErrorContext(ctx, "wiring", "err", "dump-dir cannot be set to empty string if specified")
					os.Exit(1)
				}
			}

			err = createExtractions(inputDir, doclingURL, dbPath, dumpDir, logger, ctx)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}
		},
	}

	flags := extractCmd.Flags()
	flags.StringP("input-dir", "i", "", "-i | --input-dir /path/to/input-dir | ./input-dir")
	flags.StringP("dump-dir", "d", "", "-d | --dump-dir /where/to/dump/resp-req")

	return extractCmd
}

func createExtractions(inputDir, doclingURL, dbPath, dumpDir string, logger *slog.Logger, ctx context.Context) error {
	sourceDocSource, err := source.NewSourceDocSource(inputDir, logger)
	if err != nil {
		return fmt.Errorf("error creating docs source: %s", err.Error())
	}
	db, err := sqlite.NewConn(dbPath, false)
	if err != nil {
		return err
	}

	extracedDocSink, err := extraction.NewExtractedDocSink(db, logger)
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
		client, err = httpclient.NewDump(timeout, dumpDir, logger)
		if err != nil {
			return err
		}
	} else {
		client = httpclient.New(timeout)
	}

	extractor, err := docling.NewDoclingExtractor(doclingURL, client, logger)
	if err != nil {
		return fmt.Errorf("error creating docs sink: %s", err.Error())
	}

	err = step.RunExtract(&sourceDocSource, &extracedDocSink, &extractor, logger, ctx)
	if err != nil {
		return err
	}

	return nil
}
