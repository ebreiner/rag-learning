package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"rag/internal/platform/config"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/retrieval"
	"rag/internal/platform/telemetry/logging"
	"slices"

	"github.com/spf13/cobra"
)

func NewCatalogCmd() *cobra.Command {
	catalogCmd := &cobra.Command{
		Use:   "catalog",
		Short: "Print the sources and the metadata keys and values in the DB as JSON",
		Long: `Print the sources with their document counts, and for each source the metadata
keys with their types and values, as JSON. Use the values with retrieve --source
and --meta. A key lists its values only up to 20 distinct values. --source prints
only one source.`,
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			ctx := context.Background()

			logger, cnf, otelShutdownFunc, err := Setup(ctx, cmd)
			if err != nil {
				return fmt.Errorf("error setting up otel and logger: %w", err)
			}
			logger = logger.With(logging.KeyStep, "catalog")
			defer func() {
				if err := otelShutdownFunc(ctx); err != nil {
					logger.ErrorContext(ctx, "shutdown-err", "err", fmt.Errorf("error flushing signals and shuting down otel: %w", err))
				}
			}()

			db, err := sqlite.NewConn(cnf.Storage.DBPath, false)
			if err != nil {
				return err
			}
			defer func() {
				if closeErr := db.Close(); closeErr != nil {
					err = errors.Join(err, fmt.Errorf("closing db: %w", closeErr))
				}
			}()

			catalog, err := retrieval.ReadCatalog(ctx, db)
			if err != nil {
				return fmt.Errorf("error reading catalog: %w", err)
			}

			var result any = catalog
			if source := cmd.Flag("source").Value.String(); source != "" {
				if !slices.ContainsFunc(cnf.Sources, func(s config.Source) bool { return s.Name == source }) {
					return fmt.Errorf("unknown source %q", source)
				}
				result = catalog.Source(source)
			}

			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))
			return err
		},
	}

	catalogCmd.Flags().String("source", "", "--source manual: print only this source")

	return catalogCmd
}
