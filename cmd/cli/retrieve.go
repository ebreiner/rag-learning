package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"rag/internal/platform/embedclient"
	"rag/internal/platform/sqlite/retrieval"
	"rag/internal/retrieval/step"

	"github.com/spf13/cobra"
)

var retrieveCmd = &cobra.Command{
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
			log.Fatalf("Missing query string --query 'query string'")
		}

		retrievalTypeFlag := cmd.Flag("retrieval-type")
		retrievalType := retrievalTypeFlag.Value.String()

		err := retrieveChunks(userQuery, retrievalType)
		if err != nil {
			log.Fatal(err)
		}
	},
}

func init() {
	rootCmd.AddCommand(retrieveCmd)
	retrieveCmd.Flags().StringP("query", "q", "", "-q 'alles zur farbe grün")
	retrieveCmd.Flags().StringP("retrieval-type", "t", "hybrid", "-t fts | embedding | hybrid")
}

func retrieveChunks(query, retrievalType string) error {
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
	hydrator, err := retrieval.NewChunkHydrator(ctx)
	if err != nil {
		return err
	}

	retriever, err := retrieval.NewSQLiteRetriever(ctx)
	if err != nil {
		return err
	}

	client, err := embedclient.NewKreuzbergClient()
	if err != nil {
		return err
	}

	chunks, err := step.RunRetrieval(query, strategy, 10, &hydrator, &retriever, client)
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
