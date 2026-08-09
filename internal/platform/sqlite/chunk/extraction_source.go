package chunk

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/querries"
)

type ExtractedDocSource struct {
	dbClient *sql.DB
	q        *querries.Queries
	ctx      context.Context
	lastID   int64
}

func NewExtractedDocSource(db *sql.DB, ctx context.Context) (ExtractedDocSource, error) {
	source := ExtractedDocSource{}
	source.dbClient = db
	source.ctx = ctx

	q := querries.New(source.dbClient)
	source.q = q
	source.lastID = 0

	return source, nil
}

func (e *ExtractedDocSource) NextExtraction() (step.ExtractionToChunk, error) {
	for {
		extractionToChunk := step.ExtractionToChunk{}

		docID, err := e.q.GetDocumentIDsAfterID(e.ctx, e.lastID)
		if errors.Is(err, sql.ErrNoRows) {
			return extractionToChunk, io.EOF
		}
		if err != nil {
			return extractionToChunk, fmt.Errorf("extraction source: error fetching doc ids: %w", err)
		}

		e.lastID = docID
		var rows []querries.GetLatestExtractionOfDocRow
		rows, err = e.q.GetLatestExtractionOfDoc(e.ctx, docID)
		if err != nil {
			return extractionToChunk, fmt.Errorf("error getting latest extraction for doc %d: %w", docID, err)
		}

		if len(rows) == 0 {
			// current doc exists, but no chunkable extraction rows
			// skip and continue with next doc
			log.Printf("skipping chunking of doc %d, empty nodes nothing to chunk", docID)
			continue
		}

		extractionToChunk.ParentRepresentationID = rows[0].RepresentationID

		nodeMap, err := buildMap(rows)
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

func buildMap(rows []querries.GetLatestExtractionOfDocRow) (map[string]*step.ExtractionNode, error) {
	nodeMap := make(map[string]*step.ExtractionNode)
	for _, row := range rows {
		node := &step.ExtractionNode{}
		switch row.Kind {
		case "heading":
			node.Kind = step.KindHeading
			if !row.ContentJson.Valid {
				log.Printf("heading with id '%s' has no heading content", row.NodeID)
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
				log.Printf("paragraph with id '%s' has no paragraph content", row.NodeID)
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
				log.Printf("list_item with id '%s' has no paragraph content", row.NodeID)
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
				log.Printf("table with id '%s' has no table_content", row.NodeID)
			} else {
				content := &step.TableContent{}
				if err := json.Unmarshal([]byte(row.ContentJson.String), content); err != nil {
					return nodeMap, err
				} else {
					node.Table = content
				}
			}

		case "unsupported", "caption", "footnote", "picture", "group":
			node.Kind = step.KindUnsupported
			log.Printf("unsupported kind %s", row.Kind)
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
