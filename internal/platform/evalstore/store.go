package evalstore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"rag/internal/eval/step"
)

const runFileTimeFormat = "20060102T150405Z"

func LoadGolden(path string) (step.GoldenSet, string, error) {
	var gs step.GoldenSet
	b, err := os.ReadFile(path)
	if err != nil {
		return gs, "", fmt.Errorf("reading golden set: %w", err)
	}

	sum := sha256.Sum256(b)
	if err := json.Unmarshal(b, &gs); err != nil {
		return gs, "", fmt.Errorf("parsing golden set %s: %w", path, err)
	}
	if len(gs.Queries) == 0 {
		return gs, "", fmt.Errorf("golden set %s has no queries", path)
	}

	return gs, hex.EncodeToString(sum[:]), nil
}

func Save(dir string, run step.Run) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating runs dir: %w", err)
	}

	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding run: %w", err)
	}

	path := filepath.Join(dir, run.Header.Timestamp.UTC().Format(runFileTimeFormat)+".json")
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("creating run file: %w", err)
	}

	_, err = f.Write(b)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", fmt.Errorf("writing run file: %w", err)
	}

	return path, nil
}

func Load(path string) (step.Run, error) {
	var run step.Run
	b, err := os.ReadFile(path)
	if err != nil {
		return run, fmt.Errorf("reading run file: %w", err)
	}
	if err := json.Unmarshal(b, &run); err != nil {
		return run, fmt.Errorf("parsing run file %s: %w", path, err)
	}
	return run, nil
}

func LoadLatest(dir string) (run step.Run, path string, found bool, err error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return run, "", false, nil
	}
	if err != nil {
		return run, "", false, fmt.Errorf("reading runs dir: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return run, "", false, nil
	}
	sort.Strings(names)

	path = filepath.Join(dir, names[len(names)-1])
	run, err = Load(path)
	if err != nil {
		return run, path, false, err
	}
	return run, path, true, nil
}
