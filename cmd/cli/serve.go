package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	search "rag/internal/platform/sqlite"
	"rag/internal/retrieve"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
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

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "serve hybrid retrieval mcp",
	Run: func(cmd *cobra.Command, args []string) {
		serveMCP()
	},
}

var openAPICmd = &cobra.Command{
	Use:   "open-api",
	Short: "serve hybrid retrieval as open api http",
	Long:  "this could be an open api for hybrid retrieval, but it's not implemented yet.",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("serve open-api called")
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.AddCommand(openAPICmd, mcpCmd)
}

func serveMCP() {
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

		db, err := search.NewConn()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		query := retrieve.NewQuery(ctx, db)
		query.K = uint16(k)
		query.Query = retrievalQuery
		query.Strategy = retrieve.Hybrid
		fmt.Printf("running query: %s\n", query.CutQueryString())
		result, err := query.Run()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		type ResultChunk struct {
			DocTitle string
			Content  string
			ChunkID  int64
		}
		var jsonl string
		for _, chunk := range result.Results {
			resultChunk := ResultChunk{
				DocTitle: chunk.DocTitle,
				Content:  chunk.Text,
				ChunkID:  chunk.ID,
			}
			marshalled, err := json.Marshal(resultChunk)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("error marshaling result chunk %d: %s", chunk.ID, err.Error())), nil
			}
			jsonl = jsonl + string(marshalled)
		}

		return mcp.NewToolResultText(fmt.Sprintf(jsonl)), nil
	})
	if err := httpServer.Start("0.0.0.0:8080"); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
