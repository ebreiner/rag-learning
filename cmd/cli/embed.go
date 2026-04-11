package cli

import (
	"github.com/spf13/cobra"
	"log"
	"rag/internal/embedding"
)

// embedCmd represents the embed command
var embedCmd = &cobra.Command{
	Use:   "embed",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {
		inputDirFlag := cmd.Flag("input-dir")
		outputDirFlag := cmd.Flag("output-dir")
		inputDir := inputDirFlag.Value.String()
		outputDir := outputDirFlag.Value.String()
		outputDir, err := OutputDirHelper(outputDir)
		if err != nil {
			log.Fatal(err.Error())
		}
		embed(inputDir, outputDir)

	},
}

func init() {
	rootCmd.AddCommand(embedCmd)
	embedCmd.Flags().StringP("input-dir", "i", "./data/input", "--input-dir")
	embedCmd.Flags().StringP("output-dir", "o", "./data/output", "--output-dir")
}

func embed(inputDir, outputDir string) {
	err := embedding.EmbedInputDir(inputDir, outputDir)
	if err != nil {
		log.Fatal(err.Error())
	}
}
