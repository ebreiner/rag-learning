package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func Chunk(ctx context.Context, source ExtractionSource, sink ResultSink, logger *slog.Logger) error {
	for {
		if err := processDoc(ctx, source, sink, logger); err != nil {
			if errors.Is(err, io.EOF) {
				break
			} else {
				return err
			}
		}
	}

	return nil
}

func processDoc(ctx context.Context, source ExtractionSource, sink ResultSink, logger *slog.Logger) error {
	tracer := otel.Tracer("rag-cli-sdk")
	stepCtx, stepSpan := tracer.Start(ctx, "chunk-step")
	defer stepSpan.End()

	nextExtractionCtx, nextExtractionSpan := tracer.Start(stepCtx, "next_extraction")
	defer nextExtractionSpan.End()
	extract, err := source.NextExtraction(nextExtractionCtx)
	if err != nil {
		return err
	}

	stepSpan.SetAttributes(
		attribute.Int64("doc.id", extract.DocumentID),
	)
	nextExtractionSpan.End()

	processDocCtx, processDocSpan := tracer.Start(stepCtx, "process_doc")
	defer processDocSpan.End()
	result := ChunkResult{DocumentID: extract.DocumentID}
	chunkCandidates, err := walk(processDocCtx, extract.Roots, logger)
	if err != nil {
		return err
	}
	result.ChunksToSave = append(result.ChunksToSave, mergeCandidates(processDocCtx, chunkCandidates, logger)...)
	processDocSpan.SetAttributes(attribute.Int("doc.chunks.processed", len(result.ChunksToSave)))
	processDocSpan.End()

	sinkCtx, sinkSpan := tracer.Start(stepCtx, "save_chunks")
	defer sinkSpan.End()
	err = sink.SaveChunks(sinkCtx, result)
	if err != nil {
		return err
	}
	sinkSpan.End()

	return nil
}

type chunkCandidate struct {
	Node       *ExtractionNode
	Breadcrumb string
	Text       string
}

func walk(ctx context.Context, roots []*ExtractionNode, logger *slog.Logger) ([]chunkCandidate, error) {
	var breadCrumbs []*HeadingContent
	candidates := make([]chunkCandidate, 0)

	stack := make([]*ExtractionNode, 0, len(roots))
	for i := len(roots) - 1; i >= 0; i-- {
		stack = append(stack, roots[i])
	}

	currentBreadCrumb := func() string {
		crumbSnapshot := ""
		for _, crumb := range breadCrumbs {
			if crumbSnapshot == "" {
				crumbSnapshot = crumb.Text
			} else {
				crumbSnapshot = fmt.Sprintf("%s > %s", crumbSnapshot, crumb.Text)
			}
		}

		return crumbSnapshot
	}

	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		switch node.Kind {
		case "heading":
			if node.Heading != nil && node.Heading.Level > 0 {
				if node.Heading.Text == "" {
					node.Heading.Text = "MISSING_TITLE_PLACEHOLDER"
				}

				for len(breadCrumbs) > 0 && breadCrumbs[len(breadCrumbs)-1].Level >= node.Heading.Level {
					breadCrumbs = breadCrumbs[:len(breadCrumbs)-1]
				}
				breadCrumbs = append(breadCrumbs, node.Heading)
			} else {
				logger.WarnContext(ctx, "walk-nodes", "warn", "warning: missing heading info, falling back on placeholder or skip")
			}

		case "paragraph":
			candidate := chunkCandidate{
				Node:       node,
				Breadcrumb: currentBreadCrumb(),
			}
			if node.Paragraph != nil && node.Paragraph.Text != "" {
				candidate.Text = node.Paragraph.Text
			}
			candidates = append(candidates, candidate)

		case "list":
			var parts []string
			for _, child := range node.Children {
				if child.List == nil || child.List.Text == "" {
					logger.WarnContext(ctx, "walk-nodes", "warn", "warning: list_item missing content, skipping")
					continue
				}
				parts = append(parts, child.List.Marker+" "+child.List.Text)
			}
			if len(parts) > 0 {
				text := ""
				for _, s := range parts {
					text = text + s + "\n"
				}
				candidates = append(candidates, chunkCandidate{
					Node:       node,
					Breadcrumb: currentBreadCrumb(),
					Text:       text,
				})
			}
			continue // list_items consumed dont push

		case "table":
			if node.Table == nil {
				logger.WarnContext(ctx, "walk-nodes", "warn", "empty table content, skipping node in walk")
				continue
			}

			if len(node.Table.Cells) == 0 {
				logger.WarnContext(ctx, "walk-nodes", "warn", "warning: table has no cells or header")
			}

			colHeaderByCol := make(map[int64]string)
			for _, cell := range node.Table.Cells {
				if cell.IsColumnHeader {
					if len(cell.Text) == 0 {
						logger.WarnContext(ctx, "walk-nodes", "warn", "warning: empty column header")
					}
					for i := cell.ColStart; i <= cell.ColEnd; i++ {
						colHeaderByCol[i] = cell.Text
					}
				}
			}

			table := make(map[int]map[int]string)
			for _, cell := range node.Table.Cells {
				if cell.IsColumnHeader {
					continue
				}

				for r := cell.RowStart; r <= cell.RowEnd; r++ {
					for c := cell.ColStart; c <= cell.ColEnd; c++ {
						if table[int(r)] == nil {
							table[int(r)] = map[int]string{}
						}
						table[int(r)][int(c)] = cell.Text
					}
				}
			}

			headersRow := ""
			for i := 0; i < int(node.Table.Cols); i++ {
				if len(headersRow) == 0 {
					headersRow = colHeaderByCol[int64(i)]
				} else {
					headersRow = headersRow + " | " + colHeaderByCol[int64(i)]
				}
			}
			headersRow = headersRow + "\n"

			rows := ""
			for r := 1; r < int(node.Table.Rows); r++ {
				for c := 0; c < int(node.Table.Cols); c++ {
					if c > 0 {
						rows = rows + " | "
					}
					rows = rows + table[r][c]
				}
				rows = rows + "\n"
			}
			candidate := chunkCandidate{
				Node:       node,
				Breadcrumb: currentBreadCrumb(),
				Text:       headersRow + rows,
			}
			candidates = append(candidates, candidate)

		case "unsupported":
			logger.WarnContext(ctx, "walk-nodes", "warn", "unsupported node kind, skipping")
		default:
			return candidates, fmt.Errorf("unknown node kind: %s", node.Kind)
		}

		for i := len(node.Children) - 1; i >= 0; i-- {
			stack = append(stack, node.Children[i])
		}
	}

	return candidates, nil
}

func mergeCandidates(ctx context.Context, candidates []chunkCandidate, logger *slog.Logger) []ChunkToSave {
	merged := make([]ChunkToSave, 0)
	maxBudget := 1000
	budget := maxBudget
	lastCrumb := ""
	buffer := ""
	positionCounter := int64(0)

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		merged = append(merged, ChunkToSave{
			Text:       lastCrumb + "\n\n" + buffer,
			Breadcrumb: lastCrumb,
			Position:   positionCounter,
		})
		positionCounter++
		buffer = ""
	}

	startBuffer := func(breadcrumb string) {
		budget = maxBudget - len(breadcrumb)
	}

	for _, candidate := range candidates {
		if candidate.Node.Kind == "unknown" {
			logger.WarnContext(ctx, "walk-nodes", "warn", "unknown node kind, skipping")
			continue
		}
		if candidate.Node.Kind == "unsupported" {
			logger.WarnContext(ctx, "walk-nodes", "warn", "unsupported node kind, skipping")
			continue
		}

		if candidate.Breadcrumb != lastCrumb || budget <= 0 {
			flush()
			startBuffer(candidate.Breadcrumb)
		}

		if budget-tokenCount(candidate.Text) > 0 {
			buffer = buffer + candidate.Text
			budget = budget - len(candidate.Text)
		} else if len(buffer) > 0 {
			flush()
			startBuffer(candidate.Breadcrumb)
			if len(candidate.Text) > 0 {
				buffer = buffer + candidate.Text
				budget = budget - tokenCount(candidate.Text)
			}
		}

		lastCrumb = candidate.Breadcrumb
	}

	if len(buffer) > 0 {
		flush()
	}

	return merged
}

func tokenCount(text string) int {
	return len(text)
}
