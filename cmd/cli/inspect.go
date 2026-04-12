package cli

import (
	"context"
	"fmt"
	"log"
	"rag/internal/inspect"
	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/chunk"
	"rag/internal/platform/sqlite/documents"
	"rag/internal/platform/sqlite/embedding"
	"rag/internal/platform/sqlite/extraction"
	"rag/internal/platform/sqlite/representation"
	"slices"
	"strconv"

	"github.com/spf13/cobra"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "dumps result rows a result type like chunk",
	Long: `Dump some rows of the specified result type or print stats about the result types.
	`,
	Run: func(cmd *cobra.Command, args []string) {
		typeFlag := cmd.Flag("type")
		resultType := typeFlag.Value.String()

		formatFlag := cmd.Flag("format")
		format := formatFlag.Value.String()

		limitFlag := cmd.Flag("limit")
		limitString := limitFlag.Value.String()
		limit, err := strconv.Atoi(limitString)
		if err != nil {
			log.Fatalf("error parsing limit flag to int: %s", err)
		}

		runStats, err := cmd.Flags().GetBool("stats")
		if err != nil {
			log.Fatalf(err.Error())
		}
		err = runInspect(runStats, limit, format, resultType)
		if err != nil {
			log.Fatalf("error running inspector: %s", err.Error())
		}
	},
}

func init() {
	rootCmd.AddCommand(inspectCmd)
	inspectCmd.Flags().StringP("type", "t", "chunk", "--type doc|representation|chunk|embed|extraction|extraction-node")
	inspectCmd.Flags().StringP("format", "f", "jsonl", "--format json|jsonl")
	inspectCmd.Flags().IntP("limit", "l", 25, "integer")
	inspectCmd.Flags().Bool("stats", false, "stats")
}

func runInspect(runStats bool, limit int, format string, resultType string) error {
	inspector, err := newInspector(resultType, limit, format)
	if err != nil {
		return err
	}
	if runStats {
		err = stats(inspector)
		if err != nil {
			return err
		}
	} else {
		err = dump(inspector)
		if err != nil {
			return err
		}
	}

	return nil
}

func dump(inspector inspect.Inspector) error {
	dump, err := inspect.Dump(inspector)
	if err != nil {
		return err
	}
	fmt.Print(dump)
	return nil
}

func stats(inspector inspect.Inspector) error {
	stats, err := inspect.Stats(inspector)
	if err != nil {
		return err
	}
	fmt.Print(stats)
	return nil
}

func newInspector(resultType string, limit int, format string) (inspect.Inspector, error) {
	ctx := context.Background()
	var inspector inspect.Inspector

	db, err := sqlite.NewConn()
	if err != nil {
		return inspector, err
	}
	validFormats := []string{"json", "jsonl"}
	if !slices.Contains(validFormats, format) {
		return inspector, fmt.Errorf("unknown output format '%s'", format)
	}

	switch resultType {
	case "chunk":
		inspector = chunk.NewInspector(ctx, db, limit, format)
	case "embed":
		inspector = embedding.NewInspector(ctx, db, limit, format)
	case "doc":
		inspector = documents.NewInspector(ctx, db, limit, format)
	case "representation":
		inspector = representation.NewInspector(ctx, db, limit, format)
	case "extraction":
		inspector = extraction.NewExtractionInspector(ctx, db, limit, format)
	case "extraction-node":
		inspector = extraction.NewExtractionNodeInspector(ctx, db, limit, format)
	default:
		return inspector, fmt.Errorf("unknown result type '%s' , choose one of doc\nrepresentation\nchunk\nembed\nextraction\nextraction-node", resultType)
	}

	return inspector, nil
}
