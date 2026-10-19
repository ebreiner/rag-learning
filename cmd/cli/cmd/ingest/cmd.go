package ingest

import (
	"context"
	"fmt"
	ragcmd "rag/cmd/cli/cmd"

	"github.com/spf13/cobra"
)

func NewIngestCommand() *cobra.Command {
	ingestCmd := &cobra.Command{
		Use:   "ingest",
		Short: "Extract, chunk and embed all sources from the config into the DB",
		Long: `Write the sources from the config into the DB, then run extract, chunk and
embed in sequence. Each step skips work that is done: extract skips a known
file hash, chunk skips a chunked document, embed skips a chunk with a vector.

Use --rechunk after a change of the chunker. It deletes all chunks and
embeddings, keeps the extractions, and thus does not send the files to docling again.

Restart serve after ingest.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			logger, cnf, otelShutdownFunc, err := ragcmd.Setup(ctx, cmd)
			if err != nil {
				return fmt.Errorf("error setting up otel and logger: %w", err)
			}
			logger = logger.With("cmd", "ingest")
			defer func() {
				if err := otelShutdownFunc(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			rechunk, err := cmd.Flags().GetBool("rechunk")
			if err != nil {
				return err
			}
			if err := ingest(ctx, logger, cnf, rechunk); err != nil {
				return err
			}

			return nil
		},
	}

	ingestCmd.Flags().Bool("rechunk", false, "delete all chunks and embeddings, keep the extractions, chunk and embed again")

	return ingestCmd
}
