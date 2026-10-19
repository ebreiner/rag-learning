package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"rag/internal/platform/telemetry/logging"
	"rag/internal/retrieval/step"
	"strconv"
	"strings"

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

			filter, err := filterFromFlags(cmd)
			if err != nil {
				return err
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

			err = retrieveChunks(ctx, deps, userQuery, "hybrid", filter)
			if err != nil {
				return fmt.Errorf("error running retrieval: %w", err)
			}

			return nil
		},
	}

	retrieveCmd.Flags().StringP("query", "q", "", "-q 'alles zur farbe grün")
	retrieveCmd.Flags().StringSlice("source", nil, "--source manual (repeat or comma list): search only in these sources")
	retrieveCmd.Flags().StringArray("meta", nil, "--meta key=value (repeat): true/false is a bool, a number is a number, else a string")

	return retrieveCmd
}

func filterFromFlags(cmd *cobra.Command) (step.Filter, error) {
	var filter step.Filter

	sources, err := cmd.Flags().GetStringSlice("source")
	if err != nil {
		return filter, err
	}
	filter.Sources = sources

	metas, err := cmd.Flags().GetStringArray("meta")
	if err != nil {
		return filter, err
	}
	for _, m := range metas {
		key, raw, ok := strings.Cut(m, "=")
		if !ok || key == "" {
			return filter, fmt.Errorf("--meta %q: want key=value", m)
		}
		if filter.Metadata == nil {
			filter.Metadata = map[string]any{}
		}
		if raw == "true" || raw == "false" {
			filter.Metadata[key] = raw == "true"
		} else if f, err := strconv.ParseFloat(raw, 64); err == nil {
			filter.Metadata[key] = f
		} else {
			filter.Metadata[key] = raw
		}
	}

	return filter, nil
}

func retrieveChunks(ctx context.Context, deps step.RetrievalDeps, query, retrievalType string, filter step.Filter) error {
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

	chunks, err := step.RunRetrieval(ctx, query, 10, strategy, filter, deps)
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
