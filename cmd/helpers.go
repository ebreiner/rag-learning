package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

func outputDirHelper(outputDir string) (string, error) {
	if !filepath.IsAbs(outputDir) {
		cwd, _ := os.Getwd()
		outputDir = filepath.Join(cwd, outputDir)
	}
	_, err := os.Stat(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no output directory found, creating..")
			if err := os.Mkdir(outputDir, 0750); err != nil {
				return outputDir, fmt.Errorf("error creating output directory: %s", err.Error())
			}
		} else {
			return outputDir, fmt.Errorf("error checking for existence of output directory: %s", err.Error())
		}
	}
	return outputDir, nil
}
