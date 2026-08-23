package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"rag/internal/chunk/step"
	"rag/internal/platform/config"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/chunk"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/platform/telemetry/tracing"

	"github.com/spf13/cobra"
)

func NewChunkCmd() *cobra.Command {
	chunkCmd := &cobra.Command{
		Use:   "chunk",
		Short: "Chunks current input dir",
		Long:  `Chunks all files inside the input dir`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			logger, err := logging.FromCommand(cmd)
			if err != nil {
				log.Fatal(err)
			}
			logger = logger.With(logging.KeyStep, "chunk")

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

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			db, err := sqlite.NewConn(dbPath, true)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			sink, err := chunk.NewResultSink(db, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			flush, err := cmd.Flags().GetBool("flush")
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}
			if flush {
				if err := sqlite.FlushChunkTable(ctx, db); err != nil {
					logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error flushing chunks because of force flag: %w", err))
					os.Exit(1)
				}
				if err := sqlite.FlushAllEmbeddings(ctx, db); err != nil {
					logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error flushing embeddings because of force flag: %w", err))
					os.Exit(1)
				}
			}

			source, err := chunk.NewExtractedDocSource(db, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			err = step.Chunk(ctx, &source, sink, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error running chunker: %w", err))
				os.Exit(1)
			}

			return
		},
	}

	chunkCmd.Flags().Bool("flush", false, "--flush")
	return chunkCmd
}
