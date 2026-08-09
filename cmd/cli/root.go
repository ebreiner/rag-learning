package cli

import (
	"os"
	"rag/cmd/cli/cmd"
	"rag/internal/platform/config"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "rag-cli",
		Short: "A brief description of your application",
		Long: `A longer description that spans multiple lines and likely contains
examples and usage of using your application`,
	}
	config.RegisterFlags(root.PersistentFlags(), config.GlobalsList())

	return root
}

func Execute() {
	rootCmd := NewRootCmd()

	rootCmd.AddCommand(
		cmd.NewExtractionCmd(),
		cmd.NewChunkCmd(),
		cmd.NewEmbedCmd(),
		cmd.NewRetrieveCmd(),
		cmd.NewServeCmd(),
	)

	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}
