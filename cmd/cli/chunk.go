package cli

import (
	"log"

	"github.com/spf13/cobra"
	"rag/internal/chunk"
)

// chunkCmd represents the chunk command
var chunkCmd = &cobra.Command{
	Use:   "chunk",
	Short: "Chunks current input dir",
	Long:  `Chunks all files inside the input dir`,
	Run: func(cmd *cobra.Command, args []string) {
		inputDirFlag := cmd.Flag("input-dir")
		outputDirFlag := cmd.Flag("output-dir")
		inputDir := inputDirFlag.Value.String()
		outputDir := outputDirFlag.Value.String()
		outputDir, err := OutputDirHelper(outputDir)
		if err != nil {
			log.Fatal(err.Error())
		}

		if err := runChunk(inputDir, outputDir); err != nil {
			log.Fatal(err.Error())
		}
	},
}

func init() {
	rootCmd.AddCommand(chunkCmd)
	chunkCmd.Flags().StringP("input-dir", "i", "./data/input", "--input-dir")
	chunkCmd.Flags().StringP("output-dir", "o", "./data/output", "--output-dir")
}

func runChunk(inputDir, outputDir string) error {
	if err := chunk.Chunk(inputDir, outputDir); err != nil {
		return err
	}
	return nil
}
