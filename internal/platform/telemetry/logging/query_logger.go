package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

type QueryLogger struct {
	logger *slog.Logger
}

type Chunk struct {
	ID    int64
	Score float64
}

type QueryLog struct {
	Session  string
	Duration time.Duration
	Query    string
	Strategy string
	K        int64
	Sources  []string
	Metadata map[string]any
	Chunks   []Chunk
}

func NewQueryLogger(opts ...Option) (*QueryLogger, error) {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.logPath == "" {
		return &QueryLogger{logger: slog.New(slog.DiscardHandler)}, nil
	}

	f, err := os.OpenFile(cfg.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening query log file %q: %w", cfg.logPath, err)
	}

	handlers := []slog.Handler{
		slog.NewJSONHandler(f, nil),
	}
	return &QueryLogger{logger: slog.New(newMultiHandler(handlers...))}, nil
}

type ToolCallLog struct {
	Session  string
	Tool     string
	Args     map[string]any
	Result   string
	Chars    int
	Duration time.Duration
}

func (q *QueryLogger) LogQuery(ctx context.Context, queryLog QueryLog) {
	q.logger.InfoContext(ctx, "query",
		slog.String("session", queryLog.Session),
		slog.Int64("duration_ms", queryLog.Duration.Milliseconds()),
		slog.String("query", queryLog.Query),
		slog.String("strategy", queryLog.Strategy),
		slog.Int64("k", queryLog.K),
		slog.Any("filter_sources", queryLog.Sources),
		slog.Any("filter_metadata", queryLog.Metadata),
		slog.Any("chunks", queryLog.Chunks),
	)
}

func (q *QueryLogger) LogToolCall(ctx context.Context, call ToolCallLog) {
	q.logger.InfoContext(ctx, "tool_call",
		slog.String("session", call.Session),
		slog.Int64("duration_ms", call.Duration.Milliseconds()),
		slog.String("tool", call.Tool),
		slog.Any("args", call.Args),
		slog.String("result", call.Result),
		slog.Int("chars", call.Chars),
	)
}
