package cmd

import (
	"context"
	"log"
	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/chunk"

	"github.com/spf13/cobra"
)

func NewChunkCmd() *cobra.Command {
	chunkCmd := &cobra.Command{
		Use:   "chunk",
		Short: "Chunks current input dir",
		Long:  `Chunks all files inside the input dir`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			sink, err := chunk.NewResultSink(ctx)
			if err != nil {
				log.Fatal(err)
			}
			source, err := chunk.NewExtractedDocSource(ctx)
			if err != nil {
				log.Fatal(err)
			}
			err = step.Chunk(&source, sink)
			if err != nil {
				log.Fatal(err)
			}

			return
		},
	}

	return chunkCmd
}
