package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
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

const sidecarSuffix = ".meta.json"

type inputDoc struct {
	path        string
	relPath     string
	sourceID    string
	index       int64
	metadata    map[string]any
	metadataErr error
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
		if strings.HasSuffix(strings.ToLower(path), sidecarSuffix) {
			return nil
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

	withSidecar, invalidSidecar := 0, 0
	for i, path := range paths {
		relPath, err := filepath.Rel(source.Path, path)
		if err != nil {
			return inputDocs, err
		}
		metadata, hasSidecar, metadataErr := docMetadata(source.Attributes, path+sidecarSuffix)
		if hasSidecar {
			withSidecar++
		}
		if metadataErr != nil {
			invalidSidecar++
		}
		doc := inputDoc{
			path:        path,
			relPath:     relPath,
			sourceID:    source.Name,
			index:       int64(i),
			metadata:    metadata,
			metadataErr: metadataErr,
		}
		inputDocs = append(inputDocs, doc)
	}

	logger.InfoContext(ctx, "wiring", "info", fmt.Sprintf("source %s: %d docs, %d with sidecar, %d invalid sidecars", source.Name, len(paths), withSidecar, invalidSidecar))

	return inputDocs, nil
}

func docMetadata(attributes map[string]any, sidecarPath string) (metadata map[string]any, hasSidecar bool, err error) {
	metadata = make(map[string]any, len(attributes))
	maps.Copy(metadata, attributes)

	raw, err := os.ReadFile(sidecarPath)
	if errors.Is(err, fs.ErrNotExist) {
		return metadata, false, nil
	}
	if err != nil {
		return nil, true, fmt.Errorf("reading sidecar %s: %w", sidecarPath, err)
	}

	var sidecar map[string]any
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		return nil, true, fmt.Errorf("parsing sidecar %s: %w", sidecarPath, err)
	}
	maps.Copy(metadata, sidecar)

	return metadata, true, nil
}

func (s *DocSource) NextSourceDoc() (step.SourceDoc, error) {
	doc := step.SourceDoc{}
	if s.inputIndex == int64(len(s.inputDocs)) {
		return doc, io.EOF
	}
	inputDoc := s.inputDocs[s.inputIndex]
	s.inputIndex++
	if inputDoc.metadataErr != nil {
		return doc, inputDoc.metadataErr
	}
	hash, err := calculateSHA256Content(inputDoc.Path())
	if err != nil {
		return doc, err
	}
	doc.SHA256 = hash
	doc.Name = inputDoc.Name()
	doc.SourcePath = inputDoc.Path()
	doc.RelPath = inputDoc.relPath
	doc.SourceName = inputDoc.sourceID
	doc.Metadata = inputDoc.metadata

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
