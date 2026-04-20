package sqlite

import (
	"regexp"
	"strings"
)

var punct = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
var spaces = regexp.MustCompile(`\s+`)

func NormalizeText(s string) string {
	s = strings.ToLower(s)
	s = punct.ReplaceAllString(s, " ")
	s = spaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
