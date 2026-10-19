package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"

	"github.com/spf13/cobra"
)

func NewReadCmd() *cobra.Command {
	readCmd := &cobra.Command{
		Use:   "read",
		Short: "Print the neighbours, the section or the document of a chunk as JSON",
		Long: `Print more context for a chunk, as the MCP tool read gives it: its neighbour
chunks, its section with sub-sections, its document, or a page from the chunk on.
A result larger than read.max_chars gives an outline, then a page, then a warning.`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ctx := context.Background()

			logger, cnf, otelShutdownFunc, err := Setup(ctx, cmd)
			if err != nil {
				return fmt.Errorf("error setting up otel and logger: %w", err)
			}
			logger = logger.With(logging.KeyStep, "read")
			defer func() {
				if err := otelShutdownFunc(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			chunkID, err := cmd.Flags().GetInt64("chunk")
			if err != nil {
				return err
			}
			mode, err := cmd.Flags().GetString("mode")
			if err != nil {
				return err
			}
			n, err := cmd.Flags().GetInt64("n")
			if err != nil {
				return err
			}
			filter, err := filterFromFlags(cmd)
			if err != nil {
				return err
			}

			db, err := sqlite.NewConn(cnf.Storage.DBPath, false)
			if err != nil {
				return err
			}
			defer func() {
				if closeErr := db.Close(); closeErr != nil {
					err = errors.Join(err, fmt.Errorf("closing db: %w", closeErr))
				}
			}()

			req := step.ReadRequest{ChunkID: chunkID, Mode: step.ReadMode(mode), N: n, Filter: filter}
			result, err := step.RunRead(ctx, req, NewReadDeps(db, cnf, logger))
			if err != nil {
				return err
			}

			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return err
		},
	}

	readCmd.Flags().Int64("chunk", 0, "--chunk 679: the ID of a chunk from a retrieve result")
	readCmd.Flags().String("mode", "section", "--mode neighbours|section|document|page")
	readCmd.Flags().Int64("n", 0, "--n 2: only for neighbours, chunks before and after (1 to 5, default 2)")
	readCmd.Flags().StringSlice("source", nil, "--source manual: read only if the document is in these sources")
	readCmd.Flags().StringArray("meta", nil, "--meta key=value: read only if the document metadata has this value")

	return readCmd
}
