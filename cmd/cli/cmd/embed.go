package cmd

import (
	"context"
	"log"
	"rag/internal/embedding/step"
	"rag/internal/platform/config"
	"rag/internal/platform/embedclient/kreuzberg"
	"rag/internal/platform/embedclient/openai"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/embedding"
	"strconv"
	"time"

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
			openAIBaseURL, err := config.ResolveGlobal(cmd, globals.OpenAIEmbedURL)
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
			sink, err := embedding.NewEmbeddingsResultSink(db, ctx)
			if err != nil {
				log.Fatal(err.Error())
			}
			modelFlag := cmd.Flags().Lookup("model")
			if !modelFlag.Changed || modelFlag.Value.String() == "" {
				log.Fatal("missing flag required flag: --model model-name")
			}
			model := modelFlag.Value.String()

			dimFlag := cmd.Flags().Lookup("dim")
			if !dimFlag.Changed || dimFlag.Value.String() == "" {
				log.Fatal("missing flag required flag: --dimension 1024")
			}

			dim, err := strconv.Atoi(dimFlag.Value.String())
			if err != nil {
				log.Fatalf("error parsing flag --dimension: %s", err.Error())
			}
			tableName, err := sqlite.SetupVecTable(db, ctx, int64(dim), model)
			if err != nil {
				log.Fatal(err)
			}

			chunkSource, err := embedding.NewChunkSource(db, ctx, tableName)
			source := &chunkSource
			if err != nil {
				log.Fatal(err.Error())
			}

			var embedClient step.EmbedClient
			httpClient := httpclient.New(time.Minute * 5)
			if cmd.Flags().Lookup("xberg-url").Changed {
				if client, err := kreuzberg.NewKreuzbergClient(xbergBaseURL); err == nil {
					embedClient = client
				} else {
					log.Fatal(err)
					return
				}
			} else if cmd.Flags().Lookup("openai-url").Changed {
				if client, err := openai.NewOpenAIClient(model, openAIBaseURL, int64(dim), httpClient); err == nil {
					embedClient = client
				} else {
					log.Fatal(err)
					return
				}
			}

			err = step.Embed(sink, source, embedClient)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	embedCmd.Flags().StringP("model", "m", "", "--model | -m bge-m3")
	embedCmd.Flags().Int64P("dim", "d", -1, "--dimension | -d 768")

	return embedCmd
}
