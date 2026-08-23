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
		Run: func(cmd *cobra.Command, args []string) {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			logger, err := logging.FromCommand(cmd)
			if err != nil {
				log.Fatal(err)
			}

			shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}
			defer func() {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				if err := shutdownOTEL(shutdownCtx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
					os.Exit(1)
				}
			}()

			globals := config.GlobalOptions
			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
			}

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

			modelFlag := cmd.Flags().Lookup("model")
			if !modelFlag.Changed || modelFlag.Value.String() == "" {
				logger.ErrorContext(ctx, "wiring", "err", "missing required flag model")
				os.Exit(1)
			}

			model := modelFlag.Value.String()

			dimFlag := cmd.Flags().Lookup("dim")
			if !dimFlag.Changed || dimFlag.Value.String() == "" {
				logger.ErrorContext(ctx, "wiring", "err", "missing required flag dim")
				os.Exit(1)
			}
			dim, err := strconv.Atoi(dimFlag.Value.String())
			if err != nil {
				err = fmt.Errorf("error parsing int give for 'dim': %w", err)
				logger.ErrorContext(ctx, "wiring", "err", err)
				os.Exit(1)
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
				logger.ErrorContext(ctx, "wiring", "err", "missing required flag for embedding provider")
				os.Exit(1)
			}

			apiToken, ok := os.LookupEnv("RAG_CLI_API_TOKEN")
			if !ok {
				logger.ErrorContext(ctx, "wiring", "err", "missing required env 'RAG_CLI_API_TOKEN'")
				os.Exit(1)
			}
			if len(apiToken) == 0 {
				logger.ErrorContext(ctx, "wiring", "err", "env 'RAG_CLI_API_TOKEN' cannot be empty")
				os.Exit(1)
			}

			hydrator, retriever, embedClient, closeDB, err := wireUp(ctx, dbPath, embedConfig, logger)
			if err != nil {
				if closeDB == nil {
					logger.ErrorContext(ctx, "wiring", "err", err)
					os.Exit(1)
				} else {
					closeDB(ctx)
					logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error closing db: %w", err))
					os.Exit(1)
				}
			}

			var queryLogger *logging.QueryLogger
			queryLogger, err = logging.NewQueryLogger(logging.WithLogPath(queryLogPath))
			if err != nil {
				closeDB(ctx)
				logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error setting up querry logger: %w", err))
				os.Exit(1)
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
				logger.ErrorContext(ctx, "wiring", "err", fmt.Errorf("error running server: %w", err))
				os.Exit(1)
			}
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
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("missing sub-command 'mcp'")
		},
	}

	serveCmd.AddCommand(mcpCmd)
	mcpCmd.Flags().String("query-log", "", "--query-log ./query.log")
	mcpCmd.Flags().StringP("bind-addr", "b", "127.0.0.1:8808", "--bind-addr | -b 127.0.0.1:12345")
	mcpCmd.Flags().StringP("model", "m", "", "--model | -m bge-m3")
	mcpCmd.Flags().IntP("dim", "d", -1, "--dim | -d 1024")

	return serveCmd
}
