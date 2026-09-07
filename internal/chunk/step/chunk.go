package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"unicode"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const mergeSmallChunkThreshold = 75

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
	toSave := mergeCandidates(processDocCtx, chunkCandidates, logger)
	merged := mergeSmallChunks(toSave, mergeSmallChunkThreshold)
	logger.InfoContext(processDocCtx, "merge-small-chunks", "merged-chunks", len(toSave)-len(merged))
	processDocSpan.SetAttributes(attribute.Int("doc.chunks.small_merged", len(toSave)-len(merged)))
	result.ChunksToSave = append(result.ChunksToSave, merged...)
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
	MemberIDs  []int64 // for storing content of a container node like group or list
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
		case KindCaption, KindFootnote, KindListItem:
			parentKind, parentID := "none", "none"
			if node.Parent != nil {
				parentKind, parentID = string(node.Parent.Kind), node.Parent.ID
			}
			logger.WarnContext(ctx, "walk-nodes", "warn",
				fmt.Sprintf("%s node not consumed by its parent -- dropping. node_id=%s parent_kind=%s parent_id=%s layer=%s",
					node.Kind, node.ID, parentKind, parentID, node.Layer))
			continue

		case KindHeading:
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

		case KindParagraph:
			candidate := chunkCandidate{
				Node:       node,
				Breadcrumb: currentBreadCrumb(),
			}
			if node.Paragraph != nil && node.Paragraph.Text != "" {
				candidate.Text = node.Paragraph.Text
			}
			candidates = append(candidates, candidate)

		case KindFormula:
			candidate := chunkCandidate{
				Node:       node,
				Breadcrumb: currentBreadCrumb(),
			}
			if node.Formula != nil && node.Formula.Text != "" {
				candidate.Text = node.Formula.Text
			}
			candidates = append(candidates, candidate)
			if len(node.Children) > 0 {
				logger.WarnContext(ctx, "walk-nodes", "warn", "formula node has children")
			}
			continue

		case KindCode:
			candidate := chunkCandidate{
				Node:       node,
				Breadcrumb: currentBreadCrumb(),
			}
			if node.Code != nil && node.Code.Text != "" {
				candidate.Text = node.Code.Text
			}
			candidates = append(candidates, candidate)
			if len(node.Children) > 0 {
				logger.WarnContext(ctx, "walk-nodes", "warn", "code node has children")
			}
			continue

		case KindList:
			var parts []string
			candidate := chunkCandidate{}
			candidate.MemberIDs = make([]int64, 0)
			for _, child := range node.Children {
				if child.List == nil || child.List.Text == "" {
					logger.WarnContext(ctx, "walk-nodes", "warn", "warning: list_item missing content, skipping")
					continue
				}
				candidate.MemberIDs = append(candidate.MemberIDs, child.ExtractionNodeID)
				parts = append(parts, child.List.Marker+" "+child.List.Text)
			}
			if len(parts) > 0 {
				text := ""
				for _, s := range parts {
					text = text + s + "\n"
				}
				candidate.Breadcrumb = currentBreadCrumb()
				candidate.Node = node
				candidate.Text = text
				candidates = append(candidates, candidate)
			}
			continue // list_items consumed dont push

		case KindGroup:
			var parts []string
			candidate := chunkCandidate{}
			candidate.MemberIDs = make([]int64, 0)

			for _, child := range node.Children {
				switch child.Kind {
				case KindParagraph:
					if child.Paragraph == nil || child.Paragraph.Text == "" {
						logger.WarnContext(ctx, "walk-nodes", "warn", "warning: group item missing content, skipping")
						continue
					}
					candidate.MemberIDs = append(candidate.MemberIDs, child.ExtractionNodeID)
					parts = append(parts, child.Paragraph.Text)
				case KindUnsupported:
					logger.WarnContext(ctx, "walk-nodes", "warn", "node type unsupported as group item")
					continue
				case KindGroup:
					logger.WarnContext(ctx, "walk-nodes", "warn", "group node in group node found, unexpected!")
					continue
				default:
					logger.WarnContext(ctx, "walk-nodes", "warn", fmt.Sprintf("unsupported node kind in group node: %s", child.Kind))
				}

			}

			if len(parts) > 0 {
				text := ""
				for _, s := range parts {
					if text != "" && !unicode.IsSpace([]rune(text)[len([]rune(text))-1]) && !unicode.IsSpace([]rune(s)[0]) {
						text += " "
					}
					text = text + s
				}
				candidate.Breadcrumb = currentBreadCrumb()
				candidate.Node = node
				candidate.Text = text
				candidates = append(candidates, candidate)
			}
			continue

		case KindTable:
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

			candidate.MemberIDs = []int64{node.ExtractionNodeID}
			for _, child := range node.Children {
				switch child.Kind {
				case KindCaption:
					if child.Caption != nil && len(child.Caption.Text) > 0 {
						candidate.Text = candidate.Text + "\n" + child.Caption.Text
						candidate.MemberIDs = append(candidate.MemberIDs, child.ExtractionNodeID)
					} else {
						logger.WarnContext(ctx, "walk-nodes", "warn", "empty caption")
					}
				case KindFootnote:
					if child.Footnote != nil && len(child.Footnote.Text) > 0 {
						candidate.Text = candidate.Text + "\n" + child.Footnote.Text
						candidate.MemberIDs = append(candidate.MemberIDs, child.ExtractionNodeID)
					} else {
						logger.WarnContext(ctx, "walk-nodes", "warn", "empty footnote")
					}

				default:
					logger.WarnContext(ctx, "walk-nodes", "warn", fmt.Sprintf("table children of unsupported type: '%s'", child.Kind))
				}
			}

			candidates = append(candidates, candidate)
			continue

		case KindUnsupported:
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
	extNodeIDs := make([]ChunkExtractionNodeID, 0)
	positionCounter := int64(0)

	flush := func() {
		if len(buffer) == 0 {
			return
		}
		merged = append(merged, ChunkToSave{
			Text:              lastCrumb + "\n\n" + buffer,
			Breadcrumb:        lastCrumb,
			Position:          positionCounter,
			ExtractionNodeIDs: extNodeIDs,
			Type:              TypeContent,
		})
		positionCounter++
		buffer = ""
		extNodeIDs = make([]ChunkExtractionNodeID, 0)
	}

	startBuffer := func(breadcrumb string) {
		budget = maxBudget - len(breadcrumb)
	}

	candidatePos := int64(0)
	for _, candidate := range candidates {
		// Handle exceptions like container nodes and edge-cases
		switch candidate.Node.Kind {
		case KindUnsupported:
			logger.WarnContext(ctx, "merge-nodes", "warn", "unsupported node kind, skipping")
			continue
		case KindTable:
			if len(candidate.Text) == 0 {
				logger.WarnContext(ctx, "merge-nodes", "warn", "table node text is empty, skipping")
				continue
			}

			ids := make([]ChunkExtractionNodeID, 0)
			for pos, id := range candidate.MemberIDs {
				ids = append(ids, ChunkExtractionNodeID{ExtractionNodeID: id, Position: int64(pos)})
			}
			toSave := ChunkToSave{
				Breadcrumb:        candidate.Breadcrumb,
				Position:          positionCounter,
				ExtractionNodeIDs: ids,
				Text:              candidate.Text,
				Type:              TypeTable,
			}

			merged = append(merged, toSave)
			positionCounter++
			continue

		case KindGroup:
			if len(candidate.Text) == 0 {
				logger.WarnContext(ctx, "merge-nodes", "warn", "group node text is empty, skipping")
				continue
			}
			ids := make([]ChunkExtractionNodeID, 0)
			for pos, id := range candidate.MemberIDs {
				ids = append(ids, ChunkExtractionNodeID{ExtractionNodeID: id, Position: int64(pos)})
			}

			toSave := ChunkToSave{
				Breadcrumb:        candidate.Breadcrumb,
				Position:          positionCounter,
				ExtractionNodeIDs: ids,
				Text:              candidate.Text,
				Type:              TypeGeneric,
			}

			merged = append(merged, toSave)
			positionCounter++
			continue

		case KindList:
			if len(candidate.Text) == 0 {
				logger.WarnContext(ctx, "merge-nodes", "warn", "list node text is empty, skipping")
				continue
			}
			ids := make([]ChunkExtractionNodeID, 0)
			for pos, id := range candidate.MemberIDs {
				ids = append(ids, ChunkExtractionNodeID{ExtractionNodeID: id, Position: int64(pos)})
			}

			toSave := ChunkToSave{
				Breadcrumb:        candidate.Breadcrumb,
				Position:          positionCounter,
				ExtractionNodeIDs: ids,
				Text:              candidate.Text,
				Type:              TypeList,
			}

			merged = append(merged, toSave)
			positionCounter++
			continue

		case KindFormula:
			if len(candidate.Text) == 0 {
				logger.WarnContext(ctx, "merge-nodes", "warn", "formula node text is empty, skipping")
				continue
			}
			ids := []ChunkExtractionNodeID{{ExtractionNodeID: candidate.Node.ExtractionNodeID, Position: 0}}
			toSave := ChunkToSave{
				Breadcrumb:        candidate.Breadcrumb,
				Position:          positionCounter,
				ExtractionNodeIDs: ids,
				Text:              candidate.Text,
				Type:              TypeFormula,
			}

			merged = append(merged, toSave)
			positionCounter++
			continue

		case KindCode:
			if len(candidate.Text) == 0 {
				logger.WarnContext(ctx, "merge-nodes", "warn", "code node text is empty, skipping")
				continue
			}
			ids := []ChunkExtractionNodeID{{ExtractionNodeID: candidate.Node.ExtractionNodeID, Position: 0}}
			toSave := ChunkToSave{
				Breadcrumb:        candidate.Breadcrumb,
				Position:          positionCounter,
				ExtractionNodeIDs: ids,
				Text:              candidate.Text,
				Type:              TypeCode,
			}

			merged = append(merged, toSave)
			positionCounter++
			continue

		}

		if candidate.Breadcrumb != lastCrumb || budget <= 0 {
			flush()
			startBuffer(candidate.Breadcrumb)
		}

		if budget-tokenCount(candidate.Text) > 0 {
			if len(buffer) == 0 {
				buffer = candidate.Text
			} else {
				buffer = fmt.Sprintf("%s\n\n%s", buffer, candidate.Text)
			}
			extNodeIDs = append(extNodeIDs, ChunkExtractionNodeID{ExtractionNodeID: candidate.Node.ExtractionNodeID, Position: candidatePos})
			budget = budget - len(candidate.Text)
		} else if len(buffer) > 0 {
			flush()
			startBuffer(candidate.Breadcrumb)
			if len(candidate.Text) > 0 {
				buffer = candidate.Text
				budget = budget - tokenCount(candidate.Text)
				extNodeIDs = append(extNodeIDs, ChunkExtractionNodeID{ExtractionNodeID: candidate.Node.ExtractionNodeID, Position: candidatePos})
			}
		} else if len(candidate.Text) > 0 {
			buffer = candidate.Text
			budget = budget - tokenCount(candidate.Text)
			extNodeIDs = append(extNodeIDs, ChunkExtractionNodeID{ExtractionNodeID: candidate.Node.ExtractionNodeID, Position: candidatePos})
		}

		lastCrumb = candidate.Breadcrumb
		candidatePos++
	}

	if len(buffer) > 0 {
		flush()
	}

	return merged
}

func tokenCount(text string) int {
	return len(text)
}
