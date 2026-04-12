package cli

import (
	"context"
	"log"
	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/chunk"

	"github.com/spf13/cobra"
)

var chunkCmd = &cobra.Command{
	Use:   "chunk",
	Short: "Chunks current input dir",
	Long:  `Chunks all files inside the input dir`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := runChunk(); err != nil {
			log.Fatal(err.Error())
		}
	},
}

func init() {
	rootCmd.AddCommand(chunkCmd)
}

func runChunk() error {
	ctx := context.Background()
	sink, err := chunk.NewResultSink(ctx)
	if err != nil {
		return err
	}

	source, err := chunk.NewExtractedDocSource(ctx)
	if err != nil {
		return err
	}
	err = step.Chunk(&source, sink)
	if err != nil {
		return err
	}
	return nil
}
