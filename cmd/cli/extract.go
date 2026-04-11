package cli

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"rag/internal/extract"
)

var extractCmd = &cobra.Command{
	Use:   "extract",
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

		if err := extractDir(inputDir, outputDir); err != nil {
			log.Fatal(err.Error())
		}
	},
}

func init() {
	rootCmd.AddCommand(extractCmd)
	extractCmd.Flags().StringP("input-dir", "i", "./data/input", "--input-dir")
	extractCmd.Flags().StringP("output-dir", "o", "./data/output", "--output-dir")
}

func extractDir(inputDir, outputDir string) error {
	if err := extract.Extract(outputDir, inputDir); err != nil {
		return fmt.Errorf("error running extraction for %s: %s", inputDir, err.Error())
	}

	return nil
}
