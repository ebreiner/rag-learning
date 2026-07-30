package cmd

import (
	"context"
	"log"
	"rag/internal/embedding/step"
	"rag/internal/platform/embedclient"
	"rag/internal/platform/sqlite/embedding"

	"github.com/spf13/cobra"
)

func NewEmbedCmd() *cobra.Command {
	embedCmd := &cobra.Command{
		Use:   "embed",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			ctx := context.Background()
			sink, err := embedding.NewEmbedingsResultSink(ctx)
			if err != nil {
				log.Fatal(err.Error())
			}
			chunkSource, err := embedding.NewChunkSource(ctx)
			source := &chunkSource
			if err != nil {
				log.Fatal(err.Error())
			}
			client, err := embedclient.NewKreuzbergClient()
			if err != nil {
				log.Fatal(err.Error())
			}

			err = step.Embed(sink, source, client)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	embedCmd.Flags().StringP("input-dir", "i", "./data/input", "--input-dir")
	embedCmd.Flags().StringP("output-dir", "o", "./data/output", "--output-dir")

	return embedCmd
}
