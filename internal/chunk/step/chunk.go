package step

import (
	"errors"
	"fmt"
	"io"
	"log"
)

func Chunk(source ExtractionSource, sink ResultSink) error {
	var counter int
	var outerErr error
	for {
		counter++
		extract, err := source.NextExtraction()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			fmt.Println("extraction source broke")
			outerErr = err
			break
		}

		err = processDoc(extract, sink)
		if err != nil {
			outerErr = err
			break
		}
	}

	return outerErr
}

func processDoc(extraction ExtractionToChunk, sink ResultSink) error {
	result := ChunkResult{ParentRepresentationID: extraction.ParentRepresentationID}
	chunkCandidates, err := walk(extraction.Roots)
	if err != nil {
		return err
	}
	result.ChunksToSave = append(result.ChunksToSave, mergeCandidates(chunkCandidates)...)

	if err := sink.SaveChunks(result); err != nil {
		return err
	}

	return nil
}

type chunkCandidate struct {
	Node       *ExtractionNode
	Breadcrumb string
	Text       string
}

func walk(roots []*ExtractionNode) ([]chunkCandidate, error) {
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
				log.Print("warning: missing heading info, falling back on placeholder or skip")
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
					log.Print("warning: list_item missing content, skipping")
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
				log.Print("empty table content, skipping node in walk")
				continue
			}

			if len(node.Table.Cells) == 0 {
				log.Print("warning: table has no cells or header")
			}

			colHeaderByCol := make(map[int64]string)
			for _, cell := range node.Table.Cells {
				if cell.IsColumnHeader {
					if len(cell.Text) == 0 {
						log.Print("warning: empty column header")
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
			log.Print("unsupported node kind, skipping")
		default:
			return candidates, fmt.Errorf("unknown node kind: %s", node.Kind)
		}

		for i := len(node.Children) - 1; i >= 0; i-- {
			stack = append(stack, node.Children[i])
		}
	}

	return candidates, nil
}

func mergeCandidates(candidates []chunkCandidate) []ChunkToSave {
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
			log.Print("unknown node kind, skipping")
			continue
		}
		if candidate.Node.Kind == "unsupported" {
			log.Print("unsupported node kind, skipping")
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
