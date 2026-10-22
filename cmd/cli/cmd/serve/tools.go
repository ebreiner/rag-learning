package serve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

func hyridRetrievalTool(deps step.RetrievalDeps, linkDeps step.LinkDeps, queryLogger *logging.QueryLogger) server.ServerTool {
	tool := mcp.NewTool(
		"rag",
		mcp.WithDescription(`Retrieves relevant context chunks from the application's manual using semantic and keyword search. 
		Use this tool when you need authoritative information about the application's domain, features, configuration, usage, or behavior.
		The result has the chunks and, for each document, its metadata and its linked documents, grouped by the section that contains the link.
		Open a linked document or a closed group with read and its chunk_id.`),
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
		start := time.Now()
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

		searchResult, err := step.AttachLinks(handlerCtx, chunks, filter, linkDeps)
		if err != nil {
			deps.Logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("error attaching links: %w", err))
			return mcp.NewToolResultError("error loading document links"), nil
		}

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
			Session:  sessionID(ctx),
			Duration: time.Since(start),
			Query:    query,
			K:        int64(k),
			Strategy: "hybrid",
			Sources:  filter.Sources,
			Metadata: filter.Metadata,
			Chunks:   chunksToLog,
			Links:    searchResult.CountLinks(),
		}

		queryLogger.LogQuery(logCtx, queryLog)
		handlerSpan.SetAttributes(
			attribute.Int64Slice("query.chunks.ids", chunkIDs),
			attribute.Int64Slice("query.chunks.char_counts", chunkCharCounts),
			attribute.Float64Slice("query.chunks.scores", chunkScores),
		)
		logSpan.End()

		jsonResult, err := json.Marshal(searchResult)
		if err != nil {
			deps.Logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("error marshaling the result: %w", err))
			return mcp.NewToolResultError("error marshaling the result"), nil
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
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

func newCatalogTool(catalog step.Catalog, sources []string, queryLogger *logging.QueryLogger) (server.ServerTool, error) {
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

	return server.ServerTool{Tool: tool, Handler: withToolLog("catalog", queryLogger, handler, nil)}, nil
}

func newReadTool(deps step.ReadDeps, queryLogger *logging.QueryLogger, logger *slog.Logger) server.ServerTool {
	modes := make([]string, 0, len(step.ReadModes))
	for _, m := range step.ReadModes {
		modes = append(modes, string(m))
	}

	tool := mcp.NewTool(
		"read",
		mcp.WithDescription(`Reads more context for a chunk of a rag result: its neighbour chunks, its section or its document.
		A section or document that is too large gives an outline: call read again with mode section and the chunk_id of an outline entry.
		If no outline fits, the result is a page with next_chunk_id: continue with mode page and that chunk_id.`),
		mcp.WithNumber("chunk_id", mcp.Required(), mcp.Description("the ID of a chunk from a rag result")),
		mcp.WithString("mode", mcp.Required(), mcp.Enum(modes...),
			mcp.Description("neighbours: the chunk and n chunks before and after it; section: its section with sub-sections; document: the full document; page: the document from this chunk on")),
		mcp.WithNumber("n", mcp.Description("only for mode neighbours: chunks before and after, 1 to 5, default 2")),
		mcp.WithArray("sources", mcp.WithStringEnumItems(deps.Sources), mcp.Description("optional: read only if the document is in these sources")),
		mcp.WithObject("metadata", mcp.Description("optional: read only if the document metadata has exactly these values")),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		chunkID, err := request.RequireInt("chunk_id")
		if err != nil {
			return mcp.NewToolResultError("chunk_id not found in request"), nil
		}
		mode, err := request.RequireString("mode")
		if err != nil {
			return mcp.NewToolResultError("mode not found in request"), nil
		}
		filter, err := filterFromArgs(request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		req := step.ReadRequest{ChunkID: int64(chunkID), Mode: step.ReadMode(mode), N: int64(request.GetInt("n", 0)), Filter: filter}
		result, err := step.RunRead(ctx, req, deps)
		if errors.Is(err, step.ErrInvalidRead) || errors.Is(err, step.ErrInvalidFilter) ||
			errors.Is(err, step.ErrChunkNotFound) || errors.Is(err, step.ErrChunkNotInFilter) {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if err != nil {
			logger.ErrorContext(ctx, "mcp", "err", fmt.Errorf("error running read: %w", err))
			return mcp.NewToolResultError("error running read"), nil
		}

		body, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError("error marshaling read result"), nil
		}
		return mcp.NewToolResultText(string(body)), nil
	}

	readResultType := func(text string) string {
		var result step.ReadResult
		if err := json.Unmarshal([]byte(text), &result); err != nil {
			return "unknown"
		}
		return result.Type()
	}

	return server.ServerTool{Tool: tool, Handler: withToolLog("read", queryLogger, handler, readResultType)}
}

type toolHandler = func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)

func withToolLog(tool string, queryLogger *logging.QueryLogger, handler toolHandler, resultType func(text string) string) toolHandler {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()
		result, err := handler(ctx, request)

		call := logging.ToolCallLog{Session: sessionID(ctx), Tool: tool, Args: request.GetArguments(), Result: "text", Duration: time.Since(start)}
		if result != nil && len(result.Content) > 0 {
			if text, ok := result.Content[0].(mcp.TextContent); ok {
				call.Chars = utf8.RuneCountInString(text.Text)
				if resultType != nil && !result.IsError {
					call.Result = resultType(text.Text)
				}
			}
		}
		if err != nil || result == nil || result.IsError {
			call.Result = "error"
		}
		queryLogger.LogToolCall(ctx, call)

		return result, err
	}
}

func sessionID(ctx context.Context) string {
	if session := server.ClientSessionFromContext(ctx); session != nil {
		return session.SessionID()
	}
	return ""
}
