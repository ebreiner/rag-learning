package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"rag/internal/embedding/step"
	"rag/internal/platform/config"
	"rag/internal/platform/embedclient/kreuzberg"
	"rag/internal/platform/embedclient/openai"
	"rag/internal/platform/httpclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/embedding"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/platform/telemetry/tracing"
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

			logger, err := logging.FromCommand(cmd)
			if err != nil {
				log.Fatal(err)
			}
			logger = logger.With(logging.KeyStep, "embed")

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

			sink, err := embedding.NewEmbeddingsResultSink(db, logger)
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
			tableName, err := sqlite.SetupVecTable(db, ctx, int64(dim), model)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error setting up sqlite vec tables: %w", err))
				os.Exit(1)
			}

			chunkSource, err := embedding.NewChunkSource(db, tableName, logger)
			source := &chunkSource
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

			var embedClient step.EmbedClient
			httpClient := httpclient.New(time.Minute * 30)
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

			err = step.Embed(sink, source, embedClient, ctx, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error running embedding: %w", err))
				os.Exit(1)
			}
		},
	}

	embedCmd.Flags().StringP("model", "m", "", "--model | -m bge-m3")
	embedCmd.Flags().Int64P("dim", "d", -1, "--dimension | -d 768")

	return embedCmd
}
