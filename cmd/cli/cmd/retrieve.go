package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"rag/internal/platform/config"
	"rag/internal/platform/embedclient/kreuzberg"
	"rag/internal/platform/embedclient/openai"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/retrieval"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/platform/telemetry/tracing"
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
			ctx := context.Background()
			logger, err := logging.FromCommand(cmd)
			if err != nil {
				log.Fatal(fmt.Errorf("error setting up logger: %w", err))
			}
			logger = logger.With(logging.KeyStep, "retrieval")

			shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}
			defer func() {
				if err := shutdownOTEL(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
					os.Exit(1)
				}
			}()

			userQueryFlag := cmd.Flag("query")
			userQuery := userQueryFlag.Value.String()
			if len(userQuery) == 0 {
				logger.ErrorContext(ctx, "wiring", "err", "Missing query string --query 'query string'")
				os.Exit(1)
			}

			globals := config.GlobalOptions

			xbergBaseURL, err := config.ResolveGlobal(cmd, globals.XBergURL)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			openAIBaseURL, err := config.ResolveGlobal(cmd, globals.OpenAIEmbedURL)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			db, err := sqlite.NewConn(dbPath)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			modelFlag := cmd.Flags().Lookup("model")
			if !modelFlag.Changed || modelFlag.Value.String() == "" {
				logger.ErrorContext(ctx, "wiring", "err", "missing flag required flag: --model model-name")
				os.Exit(1)
			}
			model := modelFlag.Value.String()

			dimFlag := cmd.Flags().Lookup("dim")
			if !dimFlag.Changed || dimFlag.Value.String() == "" {
				logger.ErrorContext(ctx, "wiring", "err", "missing flag required flag: --dimension 1024")
				os.Exit(1)
			}

			dim, err := strconv.Atoi(dimFlag.Value.String())
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error parsing flag --dimension: %w", err))
				os.Exit(1)
			}

			var embedClient step.EmbedClient
			httpClient := httpclient.New(time.Minute * 5)
			if cmd.Flags().Lookup("xberg-url").Changed {
				if client, err := kreuzberg.NewKreuzbergClient(xbergBaseURL, logger, httpClient); err == nil {
					embedClient = client
				} else {
					logger.ErrorContext(ctx, "wiring", "err", err)
					os.Exit(1)
				}
			} else if cmd.Flags().Lookup("openai-url").Changed {
				if client, err := openai.NewOpenAIClient(model, openAIBaseURL, int64(dim), httpClient, logger); err == nil {
					embedClient = client
				} else {
					logger.ErrorContext(ctx, "wiring", "err", err)
					os.Exit(1)
				}
			}

			retrievalTypeFlag := cmd.Flag("retrieval-type")
			retrievalType := retrievalTypeFlag.Value.String()

			err = retrieveChunks(db, embedClient, userQuery, retrievalType, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error running retrieval: %w", err))
				os.Exit(1)
			}
		},
	}

	retrieveCmd.Flags().StringP("query", "q", "", "-q 'alles zur farbe grün")
	retrieveCmd.Flags().StringP("retrieval-type", "t", "hybrid", "-t fts | embedding | hybrid")
	retrieveCmd.Flags().StringP("model", "m", "", "--model | -m bge-m3")
	retrieveCmd.Flags().Int64P("dim", "d", -1, "--dimension | -d 768")

	return retrieveCmd
}

func retrieveChunks(db *sql.DB, embedClient step.EmbedClient, query, retrievalType string, logger *slog.Logger) error {
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
	hydrator, err := retrieval.NewChunkHydrator(db, logger)
	if err != nil {
		return err

	}

	retriever, err := retrieval.NewSQLiteRetriever(db, logger)
	if err != nil {
		return err
	}

	chunks, err := step.RunRetrieval(query, strategy, 10, &hydrator, retriever, embedClient, ctx)
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
