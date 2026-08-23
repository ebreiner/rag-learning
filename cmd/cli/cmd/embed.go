package cmd

import (
	"context"
	"fmt"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			logger, err := logging.FromCommand(cmd)
			if err != nil {
				return err
			}
			logger = logger.With(logging.KeyStep, "embed")

			shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
			if err != nil {
				return fmt.Errorf("error setting up otel-sdk: %w", err)
			}
			defer func() {
				if err := shutdownOTEL(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			globals := config.GlobalOptions
			xbergBaseURL, err := config.ResolveGlobal(cmd, globals.XBergURL)
			if err != nil {
				return err
			}
			openAIBaseURL, err := config.ResolveGlobal(cmd, globals.OpenAIEmbedURL)
			if err != nil {
				return err
			}

			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				return err
			}

			db, err := sqlite.NewConn(dbPath, false)
			if err != nil {
				return err
			}

			sink, err := embedding.NewEmbeddingsResultSink(db, logger)
			if err != nil {
				return err
			}
			modelFlag := cmd.Flags().Lookup("model")
			if !modelFlag.Changed || modelFlag.Value.String() == "" {
				return fmt.Errorf("missing flag required flag: --model model-name")
			}
			model := modelFlag.Value.String()

			dimFlag := cmd.Flags().Lookup("dim")
			if !dimFlag.Changed || dimFlag.Value.String() == "" {
				return fmt.Errorf("missing flag required flag: --dimension 1024")
			}

			dim, err := strconv.Atoi(dimFlag.Value.String())
			if err != nil {
				return fmt.Errorf("error parsing flag --dimension: %w", err)
			}
			tableName, err := sqlite.SetupVecTable(ctx, db, int64(dim), model)
			if err != nil {
				return fmt.Errorf("error setting up sqlite vec tables: %w", err)
			}

			chunkSource, err := embedding.NewChunkSource(db, tableName, logger)
			source := &chunkSource
			if err != nil {
				return err
			}

			var embedClient step.EmbedClient
			httpClient := httpclient.New(time.Minute * 30)
			if cmd.Flags().Lookup("xberg-url").Changed {
				if client, err := kreuzberg.NewKreuzbergClient(xbergBaseURL, logger, httpClient); err == nil {
					embedClient = client
				} else {
					return err
				}
			} else if cmd.Flags().Lookup("openai-url").Changed {

				if client, err := openai.NewOpenAIClient(model, openAIBaseURL, int64(dim), httpClient, logger); err == nil {
					embedClient = client
				} else {
					return err
				}
			}

			err = step.Embed(ctx, sink, source, embedClient, logger)
			if err != nil {
				return err
			}

			return nil
		},
	}

	embedCmd.Flags().StringP("model", "m", "", "--model | -m bge-m3")
	embedCmd.Flags().Int64P("dim", "d", -1, "--dimension | -d 768")

	return embedCmd
}
