package step

import "strings"

func normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// true when the chunk is in the doc and each condition that is set matches
func matches(e Expectation, c Chunk) bool {
	if c.DocName != e.Doc {
		return false
	}
	if e.BreadcrumbContains != "" && !strings.Contains(normalize(c.Breadcrumb), normalize(e.BreadcrumbContains)) {
		return false
	}
	if e.TextContains != "" && !strings.Contains(normalize(c.Text), normalize(e.TextContains)) {
		return false
	}
	return true
}
