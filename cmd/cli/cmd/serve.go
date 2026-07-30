package cmd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"rag/internal/platform/config"
	"rag/internal/platform/embedclient"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/retrieval"
	"rag/internal/retrieval/step"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

func NewServeCmd() *cobra.Command {
	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "serve hybrid retrieval mcp",
		Run: func(cmd *cobra.Command, args []string) {
			globals := config.GlobalOptions
			dbPath, err := config.ResolveGlobal(cmd, globals.DBPath)
			if err != nil {
				log.Fatal(err)
			}
			db, err := sqlite.NewConn(dbPath)
			if err != nil {
				log.Fatal(err)
			}

			xbergBaseURL, err := config.ResolveGlobal(cmd, globals.XBergURL)
			if err != nil {
				log.Fatal(err)
			}

			serveMCP(db, xbergBaseURL)
		},
	}

	openAPICmd := &cobra.Command{
		Use:   "open-api",
		Short: "serve hybrid retrieval as open api http",
		Long:  "this could be an open api for hybrid retrieval, but it's not implemented yet.",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("serve open-api called")
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
			fmt.Println("call 'serve mcp' or 'serve open-api'")
		},
	}

	serveCmd.AddCommand(openAPICmd, mcpCmd)

	return serveCmd
}

func serveMCP(db *sql.DB, xbergBaseURL string) {
	s := server.NewMCPServer(
		"rag",
		"0.0.1",
		server.WithLogging(),
		server.WithToolCapabilities(false),
	)
	httpServer := server.NewStreamableHTTPServer(s)

	retrievalTool := mcp.NewTool(
		"rag",
		mcp.WithDescription(`Retrieves relevant context chunks from the application's manual using semantic and keyword search. 
		Use this tool when you need authoritative information about the application's domain, features, configuration, usage, or behavior.`),
		mcp.WithString(
			"retrieval query",
			mcp.Required(),
			mcp.Description("query for which retrieval should be performed"),
		),
		mcp.WithNumber(
			"k",
			mcp.Required(),
			mcp.Description("k for top k with k maximum of 10"),
		),
	)
	s.AddTool(retrievalTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Using helper functions for type-safe argument access
		retrievalQuery, err := request.RequireString("retrieval query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		k, err := request.RequireInt("k")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		fmt.Printf("k: %d, query: %s", k, retrievalQuery)

		hydrator, err := retrieval.NewChunkHydrator(db, ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		retriever, err := retrieval.NewSQLiteRetriever(db, ctx)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		client, err := embedclient.NewKreuzbergClient(xbergBaseURL)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		chunks, err := step.RunRetrieval(retrievalQuery, step.Hybrid, 10, &hydrator, &retriever, client)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		b, err := json.Marshal(chunks)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(string(b)), nil
	})
	if err := httpServer.Start("0.0.0.0:8080"); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
