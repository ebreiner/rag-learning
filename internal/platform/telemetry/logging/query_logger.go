package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

type QueryLogger struct {
	logger *slog.Logger
}

type QueryLog struct {
	Query    string
	Strategy string
	K        int64
	ChunkIDs []int64
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

func (q *QueryLogger) LogQuery(ctx context.Context, queryLog QueryLog) {
	q.logger.InfoContext(ctx, "query",
		slog.String("query", queryLog.Query),
		slog.String("strategy", queryLog.Strategy),
		slog.Int64("k", queryLog.K),
		slog.Any("chunk_ids", queryLog.ChunkIDs),
	)
}
