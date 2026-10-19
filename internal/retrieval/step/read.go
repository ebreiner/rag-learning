package step

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type ReadMode string

const (
	ReadNeighbours ReadMode = "neighbours"
	ReadSection    ReadMode = "section"
	ReadDocument   ReadMode = "document"
	ReadPage       ReadMode = "page"
)

var ReadModes = []ReadMode{ReadNeighbours, ReadSection, ReadDocument, ReadPage}

const (
	defaultNeighbours int64 = 2
	maxNeighbours     int64 = 5
)

var (
	ErrInvalidRead      = errors.New("invalid read request")
	ErrChunkNotFound    = errors.New("chunk not found")
	ErrChunkNotInFilter = errors.New("chunk not in the filter")
)

type DocumentLoader interface {
	DocumentOfChunk(ctx context.Context, chunkID int64, filter Filter) (LoadedDocument, error)
}

type LoadedDocument struct {
	Name     string
	Source   string
	Metadata map[string]any
	Chunks   []DocChunk
}

type DocChunk struct {
	ID         int64
	Position   int64
	Breadcrumb string
	Text       string
}

type ReadRequest struct {
	ChunkID int64
	Mode    ReadMode
	N       int64
	Filter  Filter
}

type ReadDeps struct {
	Loader   DocumentLoader
	MaxChars int64
	Sources  []string
}

type OutlineEntry struct {
	Breadcrumb string `json:"breadcrumb"`
	Chars      int    `json:"chars"`
	ChunkID    int64  `json:"chunk_id"`
}

type ReadResult struct {
	Document    string         `json:"document"`
	Source      string         `json:"source"`
	Metadata    map[string]any `json:"metadata"`
	Mode        ReadMode       `json:"mode"`
	Chars       int            `json:"chars"`
	Text        string         `json:"text,omitempty"`
	TooBig      bool           `json:"too_big"`
	Outline     []OutlineEntry `json:"outline,omitempty"`
	NextChunkID *int64         `json:"next_chunk_id,omitempty"`
	Warning     string         `json:"warning,omitempty"`
}

func RunRead(ctx context.Context, req ReadRequest, deps ReadDeps) (ReadResult, error) {
	_, span := otel.Tracer("rag-cli-sdk").Start(ctx, "step-read")
	defer span.End()
	span.SetAttributes(attribute.Int64("read.chunk_id", req.ChunkID), attribute.String("read.mode", string(req.Mode)))

	n, err := validateRead(req, deps.Sources)
	if err != nil {
		return ReadResult{}, err
	}

	doc, err := deps.Loader.DocumentOfChunk(ctx, req.ChunkID, req.Filter)
	if err != nil {
		return ReadResult{}, err
	}
	idx := -1
	for i, c := range doc.Chunks {
		if c.ID == req.ChunkID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ReadResult{}, fmt.Errorf("%w: chunk %d is not in its document", ErrChunkNotFound, req.ChunkID)
	}

	res := ReadResult{Document: doc.Name, Source: doc.Source, Metadata: doc.Metadata, Mode: req.Mode}
	limit := int(deps.MaxChars)

	switch req.Mode {
	case ReadNeighbours:
		from, to := max(0, idx-int(n)), min(len(doc.Chunks), idx+int(n)+1)
		res = fit(res, doc.Chunks[from:to], doc.Chunks, from, limit)
	case ReadSection:
		from, selection := section(doc.Chunks, idx)
		res = fit(res, selection, doc.Chunks, from, limit)
	case ReadDocument:
		res = fit(res, doc.Chunks, doc.Chunks, 0, limit)
	case ReadPage:
		res = page(res, doc.Chunks, idx, limit)
	}

	span.SetAttributes(attribute.String("read.result", res.Type()), attribute.Int("read.chars", res.Chars))

	return res, nil
}

func validateRead(req ReadRequest, sources []string) (int64, error) {
	valid := false
	for _, m := range ReadModes {
		valid = valid || req.Mode == m
	}
	if !valid {
		return 0, fmt.Errorf("%w: mode %q, permitted: %v", ErrInvalidRead, req.Mode, ReadModes)
	}
	n := req.N
	if n == 0 {
		n = defaultNeighbours
	}
	if n < 0 || n > maxNeighbours {
		return 0, fmt.Errorf("%w: n must be 1 to %d, got %d", ErrInvalidRead, maxNeighbours, req.N)
	}
	return n, req.Filter.validate(sources)
}

func section(chunks []DocChunk, idx int) (int, []DocChunk) {
	base := chunks[idx].Breadcrumb
	from := -1
	var selection []DocChunk
	for i, c := range chunks {
		if c.Breadcrumb == base || strings.HasPrefix(c.Breadcrumb, base+" > ") {
			if from < 0 {
				from = i
			}
			selection = append(selection, c)
		}
	}
	return from, selection
}

func fit(res ReadResult, selection, all []DocChunk, from, limit int) ReadResult {
	text := render(selection)
	if chars := utf8.RuneCountInString(text); chars <= limit {
		res.Text, res.Chars = text, chars
		return res
	}

	res.TooBig = true
	outline := buildOutline(selection)
	if body, err := json.Marshal(outline); err == nil && len(outline) >= 2 && utf8.RuneCount(body) <= limit {
		res.Outline = outline
		res.Chars = utf8.RuneCount(body)
		return res
	}
	return page(res, all, from, limit)
}

func page(res ReadResult, all []DocChunk, from, limit int) ReadResult {
	to := from
	for to < len(all) && utf8.RuneCountInString(render(all[from:to+1])) <= limit {
		to++
	}
	if to == from {
		res.TooBig = true
		res.Warning = "the document has too little structure to read in parts: one chunk is larger than the read limit. Use the rag tool for this document."
		return res
	}
	res.Text = render(all[from:to])
	res.Chars = utf8.RuneCountInString(res.Text)
	if to < len(all) {
		next := all[to].ID
		res.NextChunkID = &next
	}
	return res
}

func buildOutline(chunks []DocChunk) []OutlineEntry {
	outline := []OutlineEntry{}
	at := map[string]int{}
	for _, c := range chunks {
		i, ok := at[c.Breadcrumb]
		if !ok {
			i = len(outline)
			at[c.Breadcrumb] = i
			outline = append(outline, OutlineEntry{Breadcrumb: c.Breadcrumb, ChunkID: c.ID})
		}
		outline[i].Chars += utf8.RuneCountInString(body(c))
	}
	return outline
}

func render(chunks []DocChunk) string {
	var sb strings.Builder
	last := ""
	for i, c := range chunks {
		if c.Breadcrumb != "" && (i == 0 || c.Breadcrumb != last) {
			sb.WriteString("## " + c.Breadcrumb + "\n\n")
		}
		last = c.Breadcrumb
		sb.WriteString(body(c) + "\n\n")
	}
	return strings.TrimSpace(sb.String())
}

func body(c DocChunk) string {
	return strings.TrimSpace(strings.TrimPrefix(c.Text, c.Breadcrumb+"\n\n"))
}

func (r ReadResult) Type() string {
	switch {
	case r.Warning != "":
		return "warning"
	case r.Outline != nil:
		return "outline"
	case r.NextChunkID != nil:
		return "page"
	default:
		return "text"
	}
}
