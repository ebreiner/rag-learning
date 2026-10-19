package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"rag/internal/platform/config"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/platform/telemetry/tracing"

	"github.com/spf13/cobra"
)

func Setup(ctx context.Context, cmd *cobra.Command) (*slog.Logger, config.Config, func(context.Context) error, error) {
	cnfPath := cmd.Flag("config").Value.String()
	if len(cnfPath) == 0 {
		return nil, config.Config{}, nil, fmt.Errorf("no value for required flag 'config'")
	}
	cnf, err := config.Load(ctx, cnfPath)
	if err != nil {
		return nil, config.Config{}, nil, err
	}

	logger, err := logging.NewLogger(logging.WithLogPath(cnf.Log.AppLog))
	if err != nil {
		return nil, config.Config{}, nil, err
	}

	shutdownOTEL, err := tracing.SetupOTelSDK(ctx, tracing.AutarcConfig{}, logger)
	if err != nil {
		return logger, config.Config{}, nil, fmt.Errorf("error setting up otel sdk: %w", err)
	}

	return logger, cnf, shutdownOTEL, nil
}
