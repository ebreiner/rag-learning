package tracing

import (
	"context"
	"errors"
	"fmt"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/trace"
	"log/slog"
	"time"
)

type TracingConfig interface{ isTracingConfig() }

type AutarcConfig struct{}

func (AutarcConfig) isTracingConfig() {}

type OBIAttachConfig struct{}

func (OBIAttachConfig) isTracingConfig() {}

func SetupOTelSDK(ctx context.Context, cfg TracingConfig, logger *slog.Logger) (shutdown func(context.Context) error, err error) {
	logger.InfoContext(ctx, "setup-otel", "info", "starting otel setup")
	switch cfg.(type) {
	case AutarcConfig:
		var shutdownFuncs []func(context.Context) error

		shutdown = func(ctx context.Context) error {
			var err error
			for _, fn := range shutdownFuncs {
				err = errors.Join(err, fn(ctx))
			}
			shutdownFuncs = nil
			return err
		}

		// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
		handleErr := func(inErr error) error {
			return errors.Join(inErr, shutdown(ctx))
		}

		propagator := newPropagator()
		otel.SetTextMapPropagator(propagator)

		traceProvider, err := newTraceProvider(ctx)
		if err != nil {
			err = handleErr(err)
			logger.ErrorContext(ctx, "setup-otel", "err", fmt.Errorf("error setting up otel sdk: %w", err))
			return shutdown, err
		}
		shutdownFuncs = append(shutdownFuncs, traceProvider.Shutdown)
		otel.SetTracerProvider(traceProvider)

		logger.InfoContext(ctx, "setup-otel", "info", "finished setting up otel sdk")
		return shutdown, err

	case OBIAttachConfig:
		logger.InfoContext(ctx, "setup-otel", "info", "finished skipping otel sdk setup because of OBI mode")
		// Deliberately don't setup anything because we are "attached" to the
		// OBI, it's wiring us up. see https://opentelemetry.io/docs/zero-code/obi/
		return func(context.Context) error { return nil }, nil

	default:
		return nil, fmt.Errorf("unknown tracing config")
	}
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider(ctx context.Context) (*trace.TracerProvider, error) {
	exporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return nil, err
	}
	provider := trace.NewTracerProvider(
		trace.WithBatcher(exporter, trace.WithBatchTimeout(time.Second*2)),
	)

	return provider, nil
}
