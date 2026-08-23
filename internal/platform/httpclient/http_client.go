package httpclient

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type capturingTransport struct {
	next    http.RoundTripper
	dumpDir string
	logger  *slog.Logger
}

func (t *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	timestamp := time.Now().Format("2006-01-02T15-04-05")

	ctx := context.Background() // until otel and log handling is done, this is the most dumb plubming possible

	reqDump, _ := httputil.DumpRequestOut(req, true)
	regFileName := fmt.Sprintf("%s-req-dump", timestamp)
	regFilePath := filepath.Join(t.dumpDir, regFileName)
	reqFile, reqFileErr := os.OpenFile(regFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if reqFileErr != nil {
		t.logger.WarnContext(ctx, "http-client", "warn", fmt.Errorf("warning: req-dump failed opening file: %w", reqFileErr))
	}

	defer func() {
		if err := reqFile.Close(); err != nil {
			t.logger.ErrorContext(ctx, "close-db", "err", err)
		}
	}()

	_, reqFileErr = reqFile.Write(reqDump)

	if reqFileErr != nil {
		t.logger.WarnContext(ctx, "http-client", "warn", fmt.Errorf("warning: req-dump failed writing file: %w", reqFileErr))
	}

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	respDump, _ := httputil.DumpResponse(resp, true)

	respFileName := fmt.Sprintf("%s-resp-dump", timestamp)
	respFilePath := filepath.Join(t.dumpDir, respFileName)
	respFile, respFileErr := os.OpenFile(respFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if respFileErr != nil {
		t.logger.WarnContext(ctx, "http-client", "warn", fmt.Errorf("warning: resp-dump failed opening file: %s", reqFileErr))
	}

	defer func() {
		if err := respFile.Close(); err != nil {
			t.logger.ErrorContext(ctx, "close-db", "err", err)
		}
	}()

	_, respFileErr = respFile.Write(respDump)

	if respFileErr != nil {
		t.logger.WarnContext(ctx, "http-client", "warn", fmt.Errorf("warning: resp-dump failed writing file: %s", reqFileErr))
	}

	return resp, nil
}

func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(http.DefaultTransport)}
}

func NewDump(timeout time.Duration, dumpDir string, logger *slog.Logger) (*http.Client, error) {
	if err := os.MkdirAll(dumpDir, 0755); err != nil {
		return &http.Client{}, err
	}
	captureingT := &capturingTransport{next: http.DefaultTransport, dumpDir: dumpDir, logger: logger}
	return &http.Client{Timeout: timeout, Transport: otelhttp.NewTransport(captureingT)}, nil
}
