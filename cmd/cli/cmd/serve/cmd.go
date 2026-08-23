package serve

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"rag/internal/platform/config"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/platform/telemetry/tracing"
	"strconv"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func NewServeCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "serve retrieval via mcp",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			logger, err := logging.FromCommand(cmd)
			if err != nil {
				return err
			}

			shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
			if err != nil {
				return fmt.Errorf("error setting up otel-sdk: %w", err)
			}
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := shutdownOTEL(shutdownCtx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			globals := config.GlobalOptions
			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				return err
			}

			xbergBaseURL, err := config.ResolveGlobal(cmd, globals.XBergURL)
			if err != nil {
				return err
			}
			openAIBaseURL, err := config.ResolveGlobal(cmd, globals.OpenAIEmbedURL)
			if err != nil {
				return err
			}

			modelFlag := cmd.Flags().Lookup("model")
			if !modelFlag.Changed || modelFlag.Value.String() == "" {
				return fmt.Errorf("missing required flag model")
			}

			model := modelFlag.Value.String()

			dimFlag := cmd.Flags().Lookup("dim")
			if !dimFlag.Changed || dimFlag.Value.String() == "" {
				return fmt.Errorf("missing required flag dim")
			}
			dim, err := strconv.Atoi(dimFlag.Value.String())
			if err != nil {
				return fmt.Errorf("error parsing int give for 'dim': %w", err)
			}

			bindAddrFlag := cmd.Flags().Lookup("bind-addr")
			bindAddr := bindAddrFlag.Value.String()

			queryLogPath := cmd.Flags().Lookup("query-log").Value.String()

			var embedConfig embedBackendConfig
			if cmd.Flags().Lookup("xberg-url").Changed {
				embedConfig = xbergConfig{URL: xbergBaseURL}

			} else if cmd.Flags().Lookup("openai-url").Changed {
				embedConfig = openAIConfig{
					URL:   openAIBaseURL,
					Model: model,
					Dim:   int64(dim),
				}
			} else {
				return fmt.Errorf("missing required flag for embedding provider")
			}

			apiToken, ok := os.LookupEnv("RAG_CLI_API_TOKEN")
			if !ok {
				return fmt.Errorf("missing required env 'RAG_CLI_API_TOKEN'")
			}
			if len(apiToken) == 0 {
				return fmt.Errorf("env 'RAG_CLI_API_TOKEN' cannot be empty")
			}

			// TODO: db mit defer schließen
			hydrator, retriever, embedClient, closeDB, err := wireUp(ctx, dbPath, embedConfig, logger)
			if err != nil {
				if closeDB == nil {
					return err
				} else {
					closeDB(ctx)
					return err
				}
			}

			var queryLogger *logging.QueryLogger
			queryLogger, err = logging.NewQueryLogger(logging.WithLogPath(queryLogPath))
			if err != nil {
				closeDB(ctx)
				return fmt.Errorf("error setting up querry logger: %w", err)
			}

			hybridTool := hyridRetrievalTool(&hydrator, retriever, embedClient, logger, queryLogger)
			tools := []server.ServerTool{hybridTool}
			mcpHandler := MCPHandler(tools)
			wrappedAuth := BearerAuth(apiToken, logger)(mcpHandler)

			serverConf := serverConfig{
				Addr:         bindAddr,
				Handler:      wrappedAuth,
				Cleanup:      []func(context.Context) error{closeDB},
				TimeoutRead:  time.Second * 30,
				TimeoutWrite: time.Second * 30,
				TimeoutIdle:  time.Second * 120,
				OTelOpName:   "mcp-streamable",
			}
			server := serverConf.NewServer(logger)

			logger.InfoContext(ctx, "wiring", "info", "server started and ready for requests")
			err = server.Run(ctx)
			if err != nil {
				return fmt.Errorf("error running server: %w", err)
			}

			return nil
		},
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Println("missing sub-command 'mcp'")
			return nil
		},
	}

	serveCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().String("query-log", "", "--query-log ./query.log")
	mcpCmd.Flags().StringP("bind-addr", "b", "127.0.0.1:8808", "--bind-addr | -b 127.0.0.1:12345")
	mcpCmd.Flags().StringP("model", "m", "", "--model | -m bge-m3")
	mcpCmd.Flags().IntP("dim", "d", -1, "--dim | -d 1024")

	return serveCmd
}
