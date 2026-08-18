package serve

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Server struct {
	httpServer *http.Server
	cleanup    []func(ctx context.Context) error
	Logger     *slog.Logger
}

type serverConfig struct {
	Addr         string
	Handler      http.Handler
	TimeoutRead  time.Duration
	TimeoutWrite time.Duration
	TimeoutIdle  time.Duration
	Cleanup      []func(context.Context) error
	OTelOpName   string
}

func (c serverConfig) NewServer(logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:         c.Addr,
			Handler:      otelhttp.NewHandler(c.Handler, c.OTelOpName),
			ReadTimeout:  c.TimeoutRead,
			WriteTimeout: c.TimeoutWrite,
			IdleTimeout:  c.TimeoutIdle,
		},
		Logger:  logger,
		cleanup: c.Cleanup,
	}

}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.httpServer.ListenAndServe() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var errs []error
		for _, fn := range s.cleanup {
			errs = append(errs, fn(shutdownCtx))
		}
		errs = append(errs, s.httpServer.Shutdown(shutdownCtx))
		return errors.Join(errs...)
	}
}

func MCPHandler(tools []server.ServerTool) http.Handler {
	s := server.NewMCPServer(
		"rag",
		"0.0.1",
		server.WithLogging(),
		server.WithToolCapabilities(false),
	)

	for _, tool := range tools {
		s.AddTool(tool.Tool, tool.Handler)
	}
	streamable := server.NewStreamableHTTPServer(s)
	mux := http.NewServeMux()
	mux.Handle("/mcp", streamable)

	return mux
}
