package chunk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/querries"
)

type ExtractedDocSource struct {
	dbClient *sql.DB
	q        *querries.Queries
	lastID   int64
	Logger   *slog.Logger
}

func NewExtractedDocSource(db *sql.DB, logger *slog.Logger) (ExtractedDocSource, error) {
	source := ExtractedDocSource{}
	source.dbClient = db
	source.Logger = logger

	q := querries.New(source.dbClient)
	source.q = q
	source.lastID = 0

	return source, nil
}

func (e *ExtractedDocSource) NextExtraction(ctx context.Context) (step.ExtractionToChunk, error) {
	for {
		extractionToChunk := step.ExtractionToChunk{}

		docID, err := e.q.GetDocumentIDsAfterID(ctx, e.lastID)
		if errors.Is(err, sql.ErrNoRows) {
			return extractionToChunk, io.EOF
		}
		if err != nil {
			return extractionToChunk, fmt.Errorf("extraction source: error fetching doc ids: %w", err)
		}

		e.lastID = docID

		_, err = e.q.DocAlreadyChunked(ctx, docID)
		if err != nil {
			if !errors.Is(err, sql.ErrNoRows) {
				return extractionToChunk, err
			}
		} else {
			e.Logger.WarnContext(ctx, "next_extraction", "warn", fmt.Sprintf("doc '%d' already chunked, skipping", docID))
			continue
		}

		var rows []querries.GetLatestExtractionOfDocRow
		rows, err = e.q.GetLatestExtractionOfDoc(ctx, docID)
		if err != nil {
			return extractionToChunk, fmt.Errorf("error getting latest extraction for doc %d: %w", docID, err)
		}

		if len(rows) == 0 {
			e.Logger.WarnContext(ctx, "next_extraction", "warn", fmt.Sprintf("skipping chunking of doc %d, empty nodes nothing to chunk", docID))
			continue
		}

		extractionToChunk.DocumentID = rows[0].DocumentID

		nodeMap, err := buildMap(rows, ctx, e.Logger)
		if err != nil {
			return extractionToChunk, fmt.Errorf("error building flat node map: %w", err)
		}
		rootNodes, err := wireGraph(nodeMap, rows)
		if err != nil {
			return extractionToChunk, fmt.Errorf("error wiring flat map into graph: %w", err)
		}
		extractionToChunk.Roots = rootNodes

		return extractionToChunk, nil
	}
}

func buildMap(rows []querries.GetLatestExtractionOfDocRow, ctx context.Context, logger *slog.Logger) (map[string]*step.ExtractionNode, error) {
	nodeMap := make(map[string]*step.ExtractionNode)
	for _, row := range rows {
		node := &step.ExtractionNode{}
		switch row.Kind {
		case "heading":
			node.Kind = step.KindHeading
			if !row.ContentJson.Valid {
				logger.WarnContext(ctx, "next_extraction", "warn", fmt.Sprintf("heading with id '%s' has no heading content", row.NodeID))
			} else {
				content := &step.HeadingContent{}
				if err := json.Unmarshal([]byte(row.ContentJson.String), content); err != nil {
					return nodeMap, err
				} else {
					node.Heading = content
				}

			}
		case "paragraph":
			node.Kind = step.KindParagraph
			if !row.ContentJson.Valid {
				logger.WarnContext(ctx, "next_extraction", "warn", fmt.Sprintf("paragraph with id '%s' has no paragraph content", row.NodeID))
			} else {
				content := &step.ParagraphContent{}
				if err := json.Unmarshal([]byte(row.ContentJson.String), content); err != nil {
					return nodeMap, err
				} else {
					node.Paragraph = content
				}
			}
		case "list":
			node.Kind = step.KindList

		case "list_item":
			node.Kind = step.KindListItem

			if !row.ContentJson.Valid {
				logger.WarnContext(ctx, "next_extraction", "warn", fmt.Sprintf("list_item with id '%s' has no paragraph content", row.NodeID))
			} else {
				content := &step.ListItemContent{}
				if err := json.Unmarshal([]byte(row.ContentJson.String), content); err != nil {
					return nodeMap, err
				} else {
					node.List = content
				}
			}

		case "table":
			node.Kind = step.KindTable

			if !row.ContentJson.Valid {
				logger.WarnContext(ctx, "next_extraction", "warn", fmt.Sprintf("table with id '%s' has no table_content", row.NodeID))
			} else {
				content := &step.TableContent{}
				if err := json.Unmarshal([]byte(row.ContentJson.String), content); err != nil {
					return nodeMap, err
				} else {
					node.Table = content
				}
			}

		case "group":
			node.Kind = step.KindGroup

		case "unsupported", "caption", "footnote", "picture":
			node.Kind = step.KindUnsupported
			logger.WarnContext(ctx, "next_extraction", "warn", fmt.Sprintf("unsupported kind %s", row.Kind))
		default:
			return nodeMap, fmt.Errorf("unknown node kind '%s'", row.Kind)
		}

		switch row.Layer {
		case "furniture":
			node.Layer = step.LayerFurniture
		case "body":
			node.Layer = step.LayerBody
		default:
			return nodeMap, fmt.Errorf("unknown content layer: %s", row.Layer)
		}

		node.ID = row.NodeID
		nodeMap[node.ID] = node
	}

	return nodeMap, nil
}

// Returns only root nodes, not the full graph!
func wireGraph(nodes map[string]*step.ExtractionNode, rows []querries.GetLatestExtractionOfDocRow) ([]*step.ExtractionNode, error) {
	roots := make([]*step.ExtractionNode, 0)
	for _, row := range rows {
		node, ok := nodes[row.NodeID]
		if !ok {
			return roots, fmt.Errorf("missing node in map for row node id: %s", row.NodeID)
		}

		if !row.ParentID.Valid {
			// Empty parentID -> one of the root nodes
			roots = append(roots, node)
		} else {
			parentNode, ok := nodes[row.ParentID.String]
			if !ok {
				return roots, fmt.Errorf("missing parent node in map for row node id: %s", row.ParentID.String)
			}

			node.Parent = parentNode
			parentNode.Children = append(parentNode.Children, node)
		}
	}
	return roots, nil
}
