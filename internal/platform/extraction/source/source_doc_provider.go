package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"rag/internal/extract/step"
	"strings"
)

type DocSource struct {
	InputPaths       []string
	Index            int
	Logger           *slog.Logger
	CollectionName   string
	CollectionWeight float64
}

var allowedExtensions = map[string]struct{}{
	".pdf": {},

	// Word
	".doc": {}, ".dot": {},
	".docx": {}, ".dotx": {}, ".docm": {}, ".dotm": {},

	// PowerPoint
	".ppt": {}, ".pot": {}, ".pps": {},
	".pptx": {}, ".potx": {}, ".ppsx": {}, ".pptm": {}, ".potm": {}, ".ppsm": {},

	// OpenDocument text
	".odt": {}, ".ott": {},

	// plain text / markdown
	".md": {}, ".txt": {}, ".text": {},

	// markup / typesetting
	".html": {}, ".htm": {}, ".xhtml": {},
	".adoc": {}, ".asciidoc": {}, ".asc": {},
	".tex": {}, ".latex": {},
}

func NewSourceDocSource(ctx context.Context, inputPath string, collWeight float64, collName string, logger *slog.Logger) (DocSource, error) {
	source := DocSource{Logger: logger}
	if !filepath.IsAbs(inputPath) {
		cwd, _ := os.Getwd()
		inputPath = filepath.Join(cwd, inputPath)
	}

	var paths []string
	err := filepath.WalkDir(inputPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return fs.SkipDir
		}
		if _, ok := allowedExtensions[filepath.Ext(strings.ToLower(path))]; ok {
			paths = append(paths, path)
		} else {
			logger.WarnContext(ctx, "wiring", "warn", fmt.Sprintf("skipping doc extension not supported: %s", path))
		}
		return nil
	})
	if err != nil {
		return source, err
	}

	source.InputPaths = paths
	source.Index = 0
	source.CollectionName = collName
	source.CollectionWeight = collWeight

	return source, nil
}

func (s *DocSource) NextSourceDoc() (step.SourceDoc, error) {
	doc := step.SourceDoc{}
	if s.Index == len(s.InputPaths) {
		return doc, io.EOF
	}
	path := s.InputPaths[s.Index]
	_, name := filepath.Split(path)
	s.Index++
	hash, err := calculateSHA256Content(path)
	if err != nil {
		return doc, err
	}
	doc.SHA256 = hash
	doc.Name = name
	doc.SourcePath = path
	doc.CollectionName = s.CollectionName
	doc.CollectionWeight = s.CollectionWeight

	return doc, nil
}

func calculateSHA256Content(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	_, err = io.Copy(hasher, file)
	if err != nil {
		return "", err
	}

	if err := file.Close(); err != nil {
		return "", fmt.Errorf("error closing file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
