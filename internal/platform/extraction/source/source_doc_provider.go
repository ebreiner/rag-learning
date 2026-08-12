package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"rag/internal/extract/step"
)

type DocSource struct {
	ctx        context.Context
	InputPaths []string
	Index      int
}

func NewSourceDocSource(inputPath string, ctx context.Context) (DocSource, error) {
	source := DocSource{}
	if !filepath.IsAbs(inputPath) {
		cwd, _ := os.Getwd()
		inputPath = filepath.Join(cwd, inputPath)
	}

	var paths []string
	err := filepath.WalkDir(inputPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return source, err
	}

	source.InputPaths = paths
	source.Index = 0
	source.ctx = ctx

	return source, nil
}

func (s *DocSource) NextSourceDoc() (step.SourceDoc, error) {
	doc := step.SourceDoc{}
	if s.Index == len(s.InputPaths) {
		return doc, io.EOF
	}
	path := s.InputPaths[s.Index]
	_, name := filepath.Split(path)
	hash, err := calculateSHA256Content(path)
	if err != nil {
		return doc, err
	}
	doc.SHA256 = hash
	doc.Name = name
	doc.SourcePath = s.InputPaths[s.Index]
	s.Index++

	return doc, nil
}

func calculateSHA256Content(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	_, err = io.Copy(hasher, file)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
