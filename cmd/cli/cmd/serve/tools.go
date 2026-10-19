package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"
	"strings"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func hyridRetrievalTool(deps step.RetrievalDeps, queryLogger *logging.QueryLogger) server.ServerTool {
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
		mcp.WithArray(
			"sources",
			mcp.WithStringEnumItems(deps.Sources),
			mcp.Description("optional: search only in these sources"),
		),
		mcp.WithObject(
			"metadata",
			mcp.Description("optional: search only in documents whose metadata has exactly these values, e.g. {\"key\": true}"),
		),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
		tracer := otel.Tracer("rag-cli-sdk")
		handlerCtx, handlerSpan := tracer.Start(ctx, "mcp_query")

		defer func() {
			if r := recover(); r != nil {
				deps.Logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("panic recovery in retrieval handler: %v", r))
				handlerSpan.RecordError(fmt.Errorf("panic: %v", r))
				handlerSpan.SetStatus(codes.Error, "panic")
				result = mcp.NewToolResultError("internal error")
				err = nil
			}
			handlerSpan.End()
		}()

		query, err := request.RequireString("retrieval_query")
		if err != nil {
			deps.Logger.ErrorContext(handlerCtx, "mcp", "err", fmt.Errorf("missing or malformed retrieval_query in request: %w", err))
			return mcp.NewToolResultError("retrieval query not found in request"), nil
		}

		if len(query) == 0 {
			deps.Logger.ErrorContext(handlerCtx, "mcp", "err", "empty string received as retrieval query")
			return mcp.NewToolResultError("empty retrieval_query received"), nil
		}

		k, err := request.RequireInt("k")
		if err != nil {
			deps.Logger.ErrorContext(handlerCtx, "mcp", "err", fmt.Errorf("missing or malformed k in request: %w", err))
			return mcp.NewToolResultError("k not found in request"), nil
		}

		if k > 10 || k <= 0 {
			deps.Logger.ErrorContext(handlerCtx, "mcp", "err", "k is outside 1 and 10")
			return mcp.NewToolResultError("value for 'k' is outside 1 and 10"), nil
		}

		filter, err := filterFromArgs(request.GetArguments())
		if err != nil {
			deps.Logger.ErrorContext(handlerCtx, "mcp", "err", fmt.Errorf("invalid filter in request: %w", err))
			return mcp.NewToolResultError(err.Error()), nil
		}

		handlerSpan.SetAttributes(
			attribute.String("query.query", query),
			attribute.Int("query.k", k),
			attribute.String("query.strategy", string(step.StrategyHybrid)),
		)

		retrievalCtx, retrievalSpan := tracer.Start(handlerCtx, "run_retrieval")
		defer retrievalSpan.End()
		chunks, err := step.RunRetrieval(retrievalCtx, query, int64(k), step.StrategyHybrid, filter, deps)
		if errors.Is(err, step.ErrInvalidFilter) {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err != nil {
			deps.Logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("error running retrieval: %w", err))
			return mcp.NewToolResultError("error running chunk retrieval"), nil
		}
		retrievalSpan.End()

		logCtx, logSpan := tracer.Start(handlerCtx, "log_query")
		defer logSpan.End()
		chunksToLog := make([]logging.Chunk, 0, len(chunks))
		chunkIDs := make([]int64, 0, len(chunks))
		chunkScores := make([]float64, 0, len(chunks))
		chunkCharCounts := make([]int64, 0, len(chunks))
		for _, chunk := range chunks {
			chunksToLog = append(chunksToLog, logging.Chunk{ID: chunk.ID, Score: chunk.Score})
			chunkIDs = append(chunkIDs, chunk.ID)
			chunkScores = append(chunkScores, chunk.Score)
			chunkCharCounts = append(chunkCharCounts, int64(utf8.RuneCountInString(chunk.Text)))
		}

		queryLog := logging.QueryLog{
			Query:    query,
			K:        int64(k),
			Strategy: "hybrid",
			Sources:  filter.Sources,
			Metadata: filter.Metadata,
			Chunks:   chunksToLog,
		}

		queryLogger.LogQuery(logCtx, queryLog)
		handlerSpan.SetAttributes(
			attribute.Int64Slice("query.chunks.ids", chunkIDs),
			attribute.Int64Slice("query.chunks.char_counts", chunkCharCounts),
			attribute.Float64Slice("query.chunks.scores", chunkScores),
		)
		logSpan.End()

		jsonChunks, err := json.Marshal(chunks)
		if err != nil {
			deps.Logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("error marshaling chunks: %w", err))
			return mcp.NewToolResultError("error marshaling chunks"), nil
		}

		return mcp.NewToolResultText(string(jsonChunks)), nil
	}

	return server.ServerTool{Tool: tool, Handler: handler}
}

func filterFromArgs(args map[string]any) (step.Filter, error) {
	var filter step.Filter

	if raw, ok := args["sources"]; ok && raw != nil {
		list, ok := raw.([]any)
		if !ok {
			return filter, fmt.Errorf("sources must be an array of strings")
		}
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return filter, fmt.Errorf("sources must be an array of strings")
			}
			filter.Sources = append(filter.Sources, s)
		}
	}

	if raw, ok := args["metadata"]; ok && raw != nil {
		metadata, ok := raw.(map[string]any)
		if !ok {
			return filter, fmt.Errorf("metadata must be an object")
		}
		if len(metadata) > 0 {
			filter.Metadata = metadata
		}
	}

	return filter, nil
}

func newCatalogTool(catalog step.Catalog, sources []string) (server.ServerTool, error) {
	tool := mcp.NewTool(
		"catalog",
		mcp.WithDescription(fmt.Sprintf(`Lists the document metadata keys and values of one source, with document counts.
		Use it to find valid values for the metadata filter of the rag tool. Sources: %s.`, strings.Join(sources, ", "))),
		mcp.WithString(
			"source",
			mcp.Required(),
			mcp.Enum(sources...),
			mcp.Description("the source to list"),
		),
	)

	bodies := make(map[string]string, len(sources))
	for _, name := range sources {
		body, err := json.Marshal(catalog.Source(name))
		if err != nil {
			return server.ServerTool{}, fmt.Errorf("error marshaling catalog of source %s: %w", name, err)
		}
		bodies[name] = string(body)
	}

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		source, err := request.RequireString("source")
		if err != nil {
			return mcp.NewToolResultError("source not found in request"), nil
		}
		body, ok := bodies[source]
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("unknown source %q, known sources: %s", source, strings.Join(sources, ", "))), nil
		}
		return mcp.NewToolResultText(body), nil
	}

	return server.ServerTool{Tool: tool, Handler: handler}, nil
}
