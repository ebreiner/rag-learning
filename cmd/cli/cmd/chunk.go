package cmd

import (
	"context"
	"log"
	"rag/internal/chunk/step"
	"rag/internal/platform/config"
	"rag/internal/platform/sqlite"
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
			globals := config.GlobalOptions
			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				log.Fatal(err)
			}

			db, err := sqlite.NewConn(dbPath)
			if err != nil {
				log.Fatal(err)
			}
			sink, err := chunk.NewResultSink(db, ctx)
			if err != nil {
				log.Fatal(err)
			}
			source, err := chunk.NewExtractedDocSource(db, ctx)
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
