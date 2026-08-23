package cmd

import (
	"context"
	"fmt"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			logger, err := logging.FromCommand(cmd)
			if err != nil {
				return err
			}
			logger = logger.With(logging.KeyStep, "chunk")

			shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
			if err != nil {
				return fmt.Errorf("error setting up otel-sdk: %w", err)
			}
			defer func() {
				if err := shutdownOTEL(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			globals := config.GlobalOptions

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				return err
			}

			db, err := sqlite.NewConn(dbPath, true)
			if err != nil {
				return err
			}

			sink, err := chunk.NewResultSink(db, logger)
			if err != nil {
				return err
			}

			flush, err := cmd.Flags().GetBool("flush")
			if err != nil {
				return err
			}
			if flush {
				if err := sqlite.FlushChunkTable(ctx, db); err != nil {
					return fmt.Errorf("error flushing chunks: %w", err)
				}
				if err := sqlite.FlushAllEmbeddings(ctx, db); err != nil {
					return fmt.Errorf("error flushing embeddings_tables: %w", err)
				}
			}

			source, err := chunk.NewExtractedDocSource(db, logger)
			if err != nil {
				return err
			}

			err = step.Chunk(ctx, &source, sink, logger)
			if err != nil {
				return err
			}

			return nil
		},
	}

	chunkCmd.Flags().Bool("flush", false, "--flush")
	return chunkCmd
}
