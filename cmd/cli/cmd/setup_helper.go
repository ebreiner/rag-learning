package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/platform/telemetry/tracing"

	"github.com/spf13/cobra"
)

func Setup(ctx context.Context, cmd *cobra.Command) (*slog.Logger, func(context.Context) error, error) {
	logger, err := logging.FromCommand(cmd)
	if err != nil {
		return nil, nil, err
	}

	shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
	if err != nil {
		return logger, nil, fmt.Errorf("error setting up otel sdk: %w", err)
	}

	return logger, shutdownOTEL, nil
}
