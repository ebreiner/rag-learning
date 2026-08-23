package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func hyridRetrievalTool(hydrator step.ChunkHydrator, retriever step.TopKRetriever, embedClient step.EmbedClient, logger *slog.Logger, queryLogger *logging.QueryLogger) server.ServerTool {
	tool := mcp.NewTool(
		"rag",
		mcp.WithDescription(`Retrieves relevant context chunks from the application's manual using semantic and keyword search. 
		Use this tool when you need authoritative information about the application's domain, features, configuration, usage, or behavior.`),
		mcp.WithString(
			"retrieval_query",
			mcp.Required(),
			mcp.Description("query for which retrieval should be performed"),
		),
		mcp.WithNumber(
			"k",
			mcp.Required(),
			mcp.Description("k for top k with k maximum of 10"),
		),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		tracer := otel.Tracer("rag-cli-sdk")
		handlerCtx, handlerSpan := tracer.Start(ctx, "mcp_query")

		defer func() {
			if r := recover(); r != nil {
				logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("panic recovery in retrieval handler: %v", r))
				handlerSpan.RecordError(fmt.Errorf("panic: %v", r))
				handlerSpan.SetStatus(codes.Error, "panic")
				result = mcp.NewToolResultError("internal error")
				err = nil
			}
			handlerSpan.End()
		}()

		query, err := request.RequireString("retrieval_query")
		if err != nil {
			logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("missing or malformed retrieval_query in request: %w", err))
			return mcp.NewToolResultError("retrieval query not found in request"), nil
		}

		if len(query) == 0 {
			logger.ErrorContext(ctx, "mcp", "err", "empty string received as retrieval query")
			return mcp.NewToolResultError("empty retrieval_query received"), nil
		}

		k, err := request.RequireInt("k")
		if err != nil {
			logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("missing or malformed k in request: %w", err))
			return mcp.NewToolResultError("k not found in request"), nil
		}

		if k > 10 || k <= 0 {
			logger.ErrorContext(ctx, "mcp", "err", "k is outside 1 and 10")
			return mcp.NewToolResultError("value for 'k' is outside 1 and 10"), nil
		}

		handlerSpan.SetAttributes(
			attribute.String("query.query", query),
			attribute.Int("query.k", k),
			attribute.String("query.strategy", string(step.Hybrid)),
		)

		retrievalCtx, retrievalSpan := tracer.Start(handlerCtx, "run_retrieval")
		chunks, err := step.RunRetrieval(retrievalCtx, query, step.Hybrid, int64(k), hydrator, retriever, embedClient)
		if err != nil {
			logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("error running retrieval: %w", err))
			retrievalSpan.End()
			return mcp.NewToolResultError("error running chunk retrieval"), nil
		}
		retrievalSpan.End()

		logCtx, logSpan := tracer.Start(handlerCtx, "log_query")
		ids := make([]int64, 0, len(chunks))
		for _, chunk := range chunks {
			ids = append(ids, chunk.ID)
		}

		queryLog := logging.QueryLog{
			Query:    query,
			K:        int64(k),
			Strategy: "hybrid",
			ChunkIDs: ids,
		}

		var charCount int
		for _, chunk := range chunks {
			charCount = charCount + utf8.RuneCountInString(chunk.Text)
		}

		queryLogger.LogQuery(logCtx, queryLog)
		handlerSpan.SetAttributes(
			attribute.Int64Slice("query.chunks.ids", ids),
			attribute.Int64("query.chunks.total_chars", int64(charCount)),
		)
		logSpan.End()

		jsonChunks, err := json.Marshal(chunks)
		if err != nil {
			logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("error marshaling chunks: %w", err))
			return mcp.NewToolResultError("error marshaling chunks"), nil
		}

		return mcp.NewToolResultText(string(jsonChunks)), nil
	}

	return server.ServerTool{Tool: tool, Handler: handler}
}
