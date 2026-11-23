package step

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const (
	keyDocTitle = "doc.title"

	maxLinksInGroup = 50
	maxLinksIn      = 50
	smallDocLinks   = 10
)

type LinkLoader interface {
	LoadLinks(ctx context.Context, docIDs []int64, filter Filter) (LinkData, error)
}

type LinkDeps struct {
	Loader LinkLoader
}

type LinkData struct {
	Out      map[int64][]LinkedDoc
	In       map[int64][]LinkedDoc
	Sections map[int64][]SectionStart
}

type LinkedDoc struct {
	Section  string
	Text     string
	Name     string
	Metadata map[string]any
	ChunkID  int64
}

type SectionStart struct {
	Breadcrumb string
	ChunkID    int64
}

type SearchResult struct {
	Chunks    []RetrievedChunk          `json:"chunks"`
	Documents map[string]SearchDocument `json:"documents"`
}

type SearchDocument struct {
	Title         string         `json:"title"`
	Source        string         `json:"source"`
	Metadata      map[string]any `json:"metadata"`
	LinksOut      []LinkGroup    `json:"links_out"`
	LinksOutTotal int            `json:"links_out_total"`
	LinksIn       []LinkEntry    `json:"links_in"`
	LinksInTotal  int            `json:"links_in_total"`
}

type LinkGroup struct {
	Section string      `json:"section"`
	Total   int         `json:"total"`
	ChunkID *int64      `json:"chunk_id,omitempty"`
	Links   []LinkEntry `json:"links,omitempty"`
}

type LinkEntry struct {
	Title   string `json:"title"`
	Section string `json:"section,omitempty"`
	Text    string `json:"text,omitempty"`
	ChunkID int64  `json:"chunk_id"`
}

func AttachLinks(ctx context.Context, chunks []RetrievedChunk, filter Filter, deps LinkDeps) (SearchResult, error) {
	ctx, span := otel.Tracer("rag-cli-sdk").Start(ctx, "step-attach-links")
	defer span.End()

	result := SearchResult{Chunks: chunks, Documents: map[string]SearchDocument{}}
	if result.Chunks == nil {
		result.Chunks = []RetrievedChunk{}
	}

	var docIDs []int64
	chunksByDoc := map[int64][]RetrievedChunk{}
	for _, c := range chunks {
		if _, ok := chunksByDoc[c.DocID]; !ok {
			docIDs = append(docIDs, c.DocID)
		}
		chunksByDoc[c.DocID] = append(chunksByDoc[c.DocID], c)
	}
	if len(docIDs) == 0 {
		return result, nil
	}

	data, err := deps.Loader.LoadLinks(ctx, docIDs, filter)
	if err != nil {
		return result, err
	}

	for _, docID := range docIDs {
		first := chunksByDoc[docID][0]
		title := docTitle(first.Metadata)
		doc := SearchDocument{
			Title:    title,
			Source:   first.SourceName,
			Metadata: first.Metadata,
		}
		if doc.Title == "" {
			doc.Title = first.DocTitle
		}

		hitSections := map[string]bool{}
		for _, c := range chunksByDoc[docID] {
			hitSections[chunkSection(c.Breadcrumb, title)] = true
		}
		doc.LinksOut, doc.LinksOutTotal = groupLinksOut(data.Out[docID], data.Sections[docID], hitSections, title)
		doc.LinksIn, doc.LinksInTotal = linksIn(data.In[docID])
		result.Documents[strconv.FormatInt(docID, 10)] = doc
	}
	span.SetAttributes(attribute.Int("links.returned", result.CountLinks()))

	return result, nil
}

func (r SearchResult) CountLinks() int {
	n := 0
	for _, doc := range r.Documents {
		for _, g := range doc.LinksOut {
			n += len(g.Links)
		}
		n += len(doc.LinksIn)
	}
	return n
}

func groupLinksOut(out []LinkedDoc, sections []SectionStart, hitSections map[string]bool, title string) ([]LinkGroup, int) {
	groups := []LinkGroup{}
	index := map[string]int{}
	for _, l := range out {
		i, ok := index[l.Section]
		if !ok {
			i = len(groups)
			index[l.Section] = i
			groups = append(groups, LinkGroup{Section: l.Section})
		}
		groups[i].Total++
	}

	allOpen := len(out) <= smallDocLinks
	for i := range groups {
		if id, ok := firstChunkOfSection(sections, groups[i].Section, title); ok {
			groups[i].ChunkID = &id
		}
	}
	for _, l := range out {
		g := &groups[index[l.Section]]
		if !allOpen && !hitSections[g.Section] {
			continue
		}
		if len(g.Links) < maxLinksInGroup {
			g.Links = append(g.Links, LinkEntry{Title: linkedTitle(l), Text: l.Text, ChunkID: l.ChunkID})
		}
	}
	return groups, len(out)
}

func linksIn(in []LinkedDoc) ([]LinkEntry, int) {
	entries := make([]LinkEntry, 0, len(in))
	for _, l := range in {
		entries = append(entries, LinkEntry{Title: linkedTitle(l), Section: l.Section, Text: l.Text, ChunkID: l.ChunkID})
	}
	slices.SortStableFunc(entries, func(a, b LinkEntry) int { return strings.Compare(a.Title, b.Title) })
	if len(entries) > maxLinksIn {
		entries = entries[:maxLinksIn]
	}
	return entries, len(in)
}

func chunkSection(breadcrumb, title string) string {
	if title == "" {
		return breadcrumb
	}
	if breadcrumb == title {
		return ""
	}
	return strings.TrimPrefix(breadcrumb, title+" > ")
}

func firstChunkOfSection(sections []SectionStart, section, title string) (int64, bool) {
	for _, s := range sections {
		if chunkSection(s.Breadcrumb, title) == section {
			return s.ChunkID, true
		}
	}
	return 0, false
}

func docTitle(metadata map[string]any) string {
	title, _ := metadata[keyDocTitle].(string)
	return title
}

func linkedTitle(l LinkedDoc) string {
	if title := docTitle(l.Metadata); title != "" {
		return title
	}
	return l.Name
}
