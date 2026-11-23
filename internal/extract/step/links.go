package step

import (
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	KeyTitle = "doc.title"

	maxLinks       = 1000
	maxLinkTextLen = 300
)

func validateLinks(doc SourceDoc) ([]Link, error) {
	if doc.Links == nil {
		return nil, nil
	}
	if title, ok := doc.Metadata[KeyTitle].(string); !ok || strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("%w: a document with links must have %s as a non-empty string", ErrInvalidMetadata, KeyTitle)
	}
	if len(doc.Links) > maxLinks {
		return nil, fmt.Errorf("%w: %d links, max. %d", ErrInvalidMetadata, len(doc.Links), maxLinks)
	}

	type key struct{ to, section string }
	seen := make(map[key]bool, len(doc.Links))
	links := make([]Link, 0, len(doc.Links))
	for i, link := range doc.Links {
		if err := validateLinkTarget(link.To); err != nil {
			return nil, fmt.Errorf("%w: links[%d].to: %w", ErrInvalidMetadata, i, err)
		}
		if utf8.RuneCountInString(link.Text) > maxLinkTextLen {
			return nil, fmt.Errorf("%w: links[%d].text is longer than %d characters", ErrInvalidMetadata, i, maxLinkTextLen)
		}
		if utf8.RuneCountInString(link.Section) > maxLinkTextLen {
			return nil, fmt.Errorf("%w: links[%d].section is longer than %d characters", ErrInvalidMetadata, i, maxLinkTextLen)
		}
		if link.To == doc.RelPath {
			continue
		}
		k := key{link.To, link.Section}
		if seen[k] {
			continue
		}
		seen[k] = true
		links = append(links, link)
	}
	return links, nil
}

// validateLinkTarget accepts a clean path relative to the source root, in the form of documents.rel_path.
func validateLinkTarget(to string) error {
	switch {
	case to == "":
		return fmt.Errorf("is empty")
	case strings.HasPrefix(to, "/"):
		return fmt.Errorf("%q is absolute, use the path relative to the source root", to)
	case path.Clean(to) != to:
		return fmt.Errorf("%q is not a clean path, use %q", to, path.Clean(to))
	case to == ".." || strings.HasPrefix(to, "../"):
		return fmt.Errorf("%q is outside the source", to)
	}
	return nil
}
