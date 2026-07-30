package cmd

import (
	"context"
	"log"
	"rag/internal/embedding/step"
	"rag/internal/platform/config"
	"rag/internal/platform/embedclient"
	"rag/internal/platform/sqlite"
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

			globals := config.GlobalOptions
			xbergBaseURL, err := config.ResolveGlobal(cmd, globals.XBergURL)
			if err != nil {
				log.Fatal(err)
			}

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				log.Fatal(err)
			}

			db, err := sqlite.NewConn(dbPath)
			if err != nil {
				log.Fatal(err)
			}
			sink, err := embedding.NewEmbedingsResultSink(db, ctx)
			if err != nil {
				log.Fatal(err.Error())
			}
			chunkSource, err := embedding.NewChunkSource(db, ctx)
			source := &chunkSource
			if err != nil {
				log.Fatal(err.Error())
			}
			client, err := embedclient.NewKreuzbergClient(xbergBaseURL)
			if err != nil {
				log.Fatal(err.Error())
			}

			err = step.Embed(sink, source, client)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	return embedCmd
}
