package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"

	"github.com/spf13/cobra"
)

func NewRetrieveCmd() *cobra.Command {
	retrieveCmd := &cobra.Command{
		Use:   "retrieve",
		Short: "Run one hybrid query against the DB and print the chunks as JSON",
		Long: `Run one query with the hybrid strategy against the DB from the config and
print the top 10 chunks as JSON. The query uses the same retrieval wiring as
serve and eval, with the fusion settings from retrieval.fusion.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			logger, cnf, otelShutdownFunc, err := Setup(ctx, cmd)
			if err != nil {
				return fmt.Errorf("error setting up otel and logger: %w", err)
			}
			logger = logger.With(logging.KeyStep, "retrieval")
			defer func() {
				if err := otelShutdownFunc(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			userQueryFlag := cmd.Flag("query")
			userQuery := userQueryFlag.Value.String()
			if len(userQuery) == 0 {
				return fmt.Errorf("missing query string --query 'query string'")
			}

			deps, _, closeDB, err := WireUpRetrieval(ctx, cnf, logger)
			if closeDB != nil {
				defer func() {
					if err := closeDB(ctx); err != nil {
						logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error closing db: %w", err))
					}
				}()
			}
			if err != nil {
				return err
			}

			err = retrieveChunks(ctx, deps, userQuery, "hybrid")
			if err != nil {
				return fmt.Errorf("error running retrieval: %w", err)
			}

			return nil
		},
	}

	retrieveCmd.Flags().StringP("query", "q", "", "-q 'alles zur farbe grün")

	return retrieveCmd
}

func retrieveChunks(ctx context.Context, deps step.RetrievalDeps, query, retrievalType string) error {
	var strategy step.RetrievalStrategy
	switch retrievalType {
	case "fts":
		strategy = step.StrategyFTS
	case "hybrid":
		strategy = step.StrategyHybrid
	case "ann":
		strategy = step.StrategyANN
	default:
		return fmt.Errorf("unknown retrieval type: %s", retrievalType)
	}

	chunks, err := step.RunRetrieval(ctx, query, 10, strategy, deps)
	if err != nil {
		return err
	}

	for _, chunk := range chunks {
		s, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		fmt.Print(string(s))
	}

	return nil
}
