package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"rag/internal/platform/config"
	"rag/internal/platform/embedclient/kreuzberg"
	"rag/internal/platform/embedclient/openai"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/retrieval"
	"rag/internal/retrieval/step"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

func NewRetrieveCmd() *cobra.Command {
	retrieveCmd := &cobra.Command{
		Use:   "retrieve",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		Run: func(cmd *cobra.Command, args []string) {
			userQueryFlag := cmd.Flag("query")
			userQuery := userQueryFlag.Value.String()
			if len(userQuery) == 0 {
				log.Fatal("Missing query string --query 'query string'")
			}

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

			retrievalTypeFlag := cmd.Flag("retrieval-type")
			retrievalType := retrievalTypeFlag.Value.String()

			err = retrieveChunks(db, embedClient, userQuery, retrievalType)
			if err != nil {
				log.Fatal(err)
			}
		},
	}

	retrieveCmd.Flags().StringP("query", "q", "", "-q 'alles zur farbe grün")
	retrieveCmd.Flags().StringP("retrieval-type", "t", "hybrid", "-t fts | embedding | hybrid")
	retrieveCmd.Flags().StringP("model", "m", "", "--model | -m bge-m3")
	retrieveCmd.Flags().Int64P("dim", "d", -1, "--dimension | -d 768")

	return retrieveCmd
}

func retrieveChunks(db *sql.DB, embedClient step.EmbedClient, query, retrievalType string) error {
	var strategy step.RetrievalStrategy
	switch retrievalType {
	case "fts":
		strategy = step.FTS
	case "hybrid":
		strategy = step.Hybrid
	case "embedding":
		strategy = step.Embedding
	default:
		return fmt.Errorf("unknown retrieval type: %s", retrievalType)
	}

	ctx := context.Background()
	hydrator, err := retrieval.NewChunkHydrator(db, ctx)
	if err != nil {
		return err
	}

	retriever, err := retrieval.NewSQLiteRetriever(db, ctx)
	if err != nil {
		return err
	}

	chunks, err := step.RunRetrieval(query, strategy, 10, &hydrator, &retriever, embedClient)
	if err != nil {
		return err
	}

	for _, chunk := range chunks {
		s, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		fmt.Print(string(s))
	}

	return nil
}
