package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// OutputDirHelper makes outputDir absolute and creates it when it does not exist.
func OutputDirHelper(outputDir string) (string, error) {
	if !filepath.IsAbs(outputDir) {
		cwd, _ := os.Getwd()
		outputDir = filepath.Join(cwd, outputDir)
	}
	_, err := os.Stat(outputDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no output directory found, creating..")
			if err := os.MkdirAll(outputDir, 0750); err != nil {
				return outputDir, fmt.Errorf("error creating output directory: %s", err.Error())
			}
		} else {
			return outputDir, fmt.Errorf("error checking for existence of output directory: %s", err.Error())
		}
	}
	return outputDir, nil
}

// normalizeTitle makes a safe file name (without extension) from a page title.
// It replaces path separators, characters that some file systems refuse, and control characters with "_".
func normalizeTitle(title string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(title) {
		switch {
		case r < 0x20 || r == 0x7f:
			b.WriteRune('_')
		case strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	name := b.String()
	// A leading dot makes a hidden file, and "." or ".." are not file names.
	if strings.HasPrefix(name, ".") {
		name = "_" + name[1:]
	}
	if name == "" {
		name = "_"
	}
	// Keep the name below the usual limit of 255 bytes, with space for the extension.
	const maxLen = 200
	if len(name) > maxLen {
		cut := maxLen
		for cut > 0 && !isRuneStart(name[cut]) {
			cut--
		}
		name = name[:cut]
	}
	return name
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// readPassword reads the password from MW_PASSWORD, or asks for it on the terminal without echo.
func readPassword() string {
	if p := os.Getenv(passwordEnv); p != "" {
		return p
	}
	fmt.Print("Enter password: ")
	setEcho(false)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	setEcho(true)
	fmt.Println()
	if err != nil && line == "" {
		log.Fatalf("error reading in password from stdin: %s", err.Error())
	}
	return strings.TrimRight(line, "\r\n")
}

// setEcho switches the terminal echo with stty. It does nothing when stdin is not a terminal.
func setEcho(on bool) {
	arg := "-echo"
	if on {
		arg = "echo"
	}
	cmd := exec.Command("stty", arg)
	cmd.Stdin = os.Stdin
	_ = cmd.Run()
}
