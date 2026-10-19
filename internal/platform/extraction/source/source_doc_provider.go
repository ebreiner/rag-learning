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
	"rag/internal/platform/config"
	"strings"
)

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

type DocSource struct {
	inputDocs  []inputDoc
	Logger     *slog.Logger
	inputIndex int64
}

type inputDoc struct {
	path     string
	relPath  string
	sourceID string
	index    int64
}

func (d *inputDoc) Name() string {
	_, name := filepath.Split(d.path)
	return name
}

func (d *inputDoc) Path() string {
	return d.path
}

func NewSourceDocSource(ctx context.Context, sources []config.Source, logger *slog.Logger) (DocSource, error) {
	source := DocSource{Logger: logger, inputIndex: 0}

	inputDocs := make([]inputDoc, 0)
	for _, src := range sources {
		docs, err := inputDocsForSource(ctx, src, logger)
		if err != nil {
			return source, err
		}
		inputDocs = append(inputDocs, docs...)
	}

	source.inputDocs = inputDocs

	return source, nil
}

func inputDocsForSource(ctx context.Context, source config.Source, logger *slog.Logger) ([]inputDoc, error) {
	inputDocs := make([]inputDoc, 0)
	var paths []string
	err := filepath.WalkDir(source.Path, func(path string, d fs.DirEntry, err error) error {
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
		return inputDocs, err
	}

	for i, path := range paths {
		relPath, err := filepath.Rel(source.Path, path)
		if err != nil {
			return inputDocs, err
		}
		doc := inputDoc{
			path:     path,
			relPath:  relPath,
			sourceID: source.Name,
			index:    int64(i),
		}
		inputDocs = append(inputDocs, doc)
	}

	return inputDocs, nil
}

func (s *DocSource) NextSourceDoc() (step.SourceDoc, error) {
	doc := step.SourceDoc{}
	if s.inputIndex == int64(len(s.inputDocs)) {
		return doc, io.EOF
	}
	inputDoc := s.inputDocs[s.inputIndex]
	s.inputIndex++
	hash, err := calculateSHA256Content(inputDoc.Path())
	if err != nil {
		return doc, err
	}
	doc.SHA256 = hash
	doc.Name = inputDoc.Name()
	doc.SourcePath = inputDoc.Path()
	doc.RelPath = inputDoc.relPath
	doc.SourceName = inputDoc.sourceID

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
