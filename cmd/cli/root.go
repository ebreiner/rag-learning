package cli

import (
	"log"
	"os"
	"rag/cmd/cli/cmd"
	"rag/cmd/cli/cmd/ingest"
	"rag/cmd/cli/cmd/serve"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "rag-cli",
		Short: "Local RAG over document sources, served to agents via MCP",
		Long: `rag-cli extracts documents with docling, chunks and embeds them into a SQLite DB,
and serves hybrid retrieval (FTS + ANN) via MCP.

All settings come from one YAML file, given with --config. See config.yaml.example.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	flags := root.PersistentFlags()
	flags.StringP("config", "c", "", "--config | -c /etc/foo/config.yaml")

	return root
}

func Execute() {

	rootCmd := NewRootCmd()

	rootCmd.AddCommand(
		cmd.NewEvalCmd(),
		cmd.NewRetrieveCmd(),
		serve.NewServeCmd(),
		ingest.NewIngestCommand(),
	)

	err := rootCmd.Execute()
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
