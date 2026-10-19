package step

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

var ErrInvalidFilter = errors.New("invalid filter")

type Filter struct {
	Sources  []string       // OR
	Metadata map[string]any // AND
}

var metadataKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

func (f Filter) IsEmpty() bool {
	return len(f.Sources) == 0 && len(f.Metadata) == 0
}

func (f Filter) validate(knownSources []string) error {
	for _, s := range f.Sources {
		if !slices.Contains(knownSources, s) {
			return fmt.Errorf("%w: unknown source %q, known sources: %v", ErrInvalidFilter, s, knownSources)
		}
	}
	for key, value := range f.Metadata {
		if !metadataKeyPattern.MatchString(key) {
			return fmt.Errorf("%w: metadata key %q, permitted characters: A-Z a-z 0-9 _ . -", ErrInvalidFilter, key)
		}
		switch value.(type) {
		case string, bool, float64, int, int64:
		default:
			return fmt.Errorf("%w: metadata %q has a value of type %T, permitted: string, bool, number", ErrInvalidFilter, key, value)
		}
	}
	return nil
}
