package serve

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	cmdpkg "rag/cmd/cli/cmd"
	"rag/internal/platform/sqlite/retrieval"
	"rag/internal/platform/telemetry/logging"

	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func NewServeCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve the hybrid retrieval as an MCP tool over streamable HTTP",
		Long: `Serve the hybrid retrieval as the MCP tool "rag" over streamable HTTP on
server.listen from the config. Each request needs the bearer token from the
env RAG_CLI_API_TOKEN. Each query goes into log.query_log.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			logger, cnf, otelShutdownFunc, err := cmdpkg.Setup(ctx, cmd)
			if err != nil {
				return fmt.Errorf("error setting up otel and logger: %w", err)
			}
			logger = logger.With(logging.KeyStep, "serve-mcp")
			defer func() {
				if err := otelShutdownFunc(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			apiToken, ok := os.LookupEnv("RAG_CLI_API_TOKEN")
			if !ok {
				return fmt.Errorf("missing required env 'RAG_CLI_API_TOKEN'")
			}
			if len(apiToken) == 0 {
				return fmt.Errorf("env 'RAG_CLI_API_TOKEN' cannot be empty")
			}

			deps, db, closeDB, err := cmdpkg.WireUpRetrieval(ctx, cnf, logger)

			if err != nil {
				if closeDB == nil {
					return err
				} else {
					if closeDBErr := closeDB(ctx); closeDBErr != nil {
						return errors.Join(err, closeDBErr)
					}
					return err
				}
			}

			var queryLogger *logging.QueryLogger
			queryLogger, err = logging.NewQueryLogger(logging.WithLogPath(cnf.Log.QueryLog))
			if err != nil {
				if closeDBErr := closeDB(ctx); closeDBErr != nil {
					return errors.Join(err, closeDBErr)
				}
				return fmt.Errorf("error setting up querry logger: %w", err)
			}

			catalog, err := retrieval.ReadCatalog(ctx, db)
			if err != nil {
				if closeDBErr := closeDB(ctx); closeDBErr != nil {
					return errors.Join(err, closeDBErr)
				}
				return fmt.Errorf("error reading catalog: %w", err)
			}
			catalogTool, err := newCatalogTool(catalog, deps.Sources, queryLogger)
			if err != nil {
				if closeDBErr := closeDB(ctx); closeDBErr != nil {
					return errors.Join(err, closeDBErr)
				}
				return err
			}

			hybridTool := hyridRetrievalTool(deps, queryLogger)
			readTool := newReadTool(cmdpkg.NewReadDeps(db, cnf, logger), queryLogger, logger)
			tools := []server.ServerTool{hybridTool, catalogTool, readTool}
			mcpHandler := MCPHandler(tools)
			wrappedAuth := BearerAuth(apiToken, logger)(mcpHandler)

			serverConf := serverConfig{
				Addr:         cnf.Server.Listen,
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
		Short: "Run a server for the retrieval",
		Long: `Run a server for the retrieval. The only server type now is mcp.
The DB is read at start: restart the server after ingest.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.Println("missing sub-command 'mcp'")
			return nil
		},
	}

	serveCmd.AddCommand(mcpCmd)

	return serveCmd
}
