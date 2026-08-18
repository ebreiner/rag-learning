package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"rag/internal/platform/config"

	"github.com/spf13/cobra"
)

const (
	KeyErr      = "err"
	KeyQuery    = "query"
	KeyChunkID  = "chunk_id"
	KeyStrategy = "strategy"
	KeyStep     = "step"
	KeyNodeType = "node_type"
)

type Option func(*options)

type options struct {
	logPath string
}

func FromCommand(cmd *cobra.Command) (*slog.Logger, error) {
	globals := config.GlobalOptions

	logFile, err := config.ResolveGlobal(cmd, globals.LogPath)
	if err != nil {
		return nil, err
	}
	withPath := WithLogPath(logFile)
	logger, err := NewLogger(withPath)
	if err != nil {
		return nil, err
	} else {
		return logger, nil
	}

}

func WithLogPath(path string) Option {
	return func(o *options) { o.logPath = path }
}

func NewLogger(opts ...Option) (*slog.Logger, error) {
	cfg := options{}
	for _, opt := range opts {
		opt(&cfg)
	}

	handlers := []slog.Handler{
		slog.NewJSONHandler(os.Stdout, nil),
	}

	if cfg.logPath != "" {
		f, err := os.OpenFile(cfg.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("opening log file %q: %w", cfg.logPath, err)
		}
		handlers = append(handlers, slog.NewJSONHandler(f, nil))
	}

	return slog.New(newMultiHandler(handlers...)), nil
}

type multiHandler struct {
	handlers []slog.Handler
}

func newMultiHandler(handlers ...slog.Handler) *multiHandler {
	return &multiHandler{handlers: handlers}
}

func (m *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m.handlers {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m *multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, h := range m.handlers {
		if h.Enabled(ctx, record.Level) {
			if err := h.Handle(ctx, record.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithAttrs(attrs)
	}
	return newMultiHandler(next...)
}

func (m *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, len(m.handlers))
	for i, h := range m.handlers {
		next[i] = h.WithGroup(name)
	}
	return newMultiHandler(next...)
}
