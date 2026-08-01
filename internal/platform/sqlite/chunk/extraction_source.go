package chunk

import (
	"context"
	"database/sql"
	"fmt"
	"io"
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

		docIDParam := querries.GetDocumentIDsAfterIDParams{
			ID:    e.lastID,
			Limit: 1,
		}

		docIDs, err := e.q.GetDocumentIDsAfterID(e.ctx, docIDParam)
		if err != nil {
			return extractionToChunk, fmt.Errorf("extraction source: error fetching doc ids: %w", err)
		}
		if len(docIDs) == 0 {
			return extractionToChunk, io.EOF
		}
		if len(docIDs) > 1 {
			return extractionToChunk, fmt.Errorf("expected exactly one doc id, got %d", len(docIDs))
		}

		docID := docIDs[0]
		e.lastID = docID

		rows, err := e.q.GetLatestExtractionOfDoc(e.ctx, docID)
		if err != nil {
			return extractionToChunk, fmt.Errorf("error getting latest extraction for doc %d: %w", docID, err)
		}
		if len(rows) == 0 {
			// current doc exists, but no chunkable extraction rows
			// skip and continue with next doc
			fmt.Printf("skipping doc %d because it has no extraction nodes", docID)
			continue
		}

		extractionToChunk.ParentRepresentationID = rows[0].RepresentationID
		nodes := make([]step.ExtractionNode, 0, len(rows))
		for _, row := range rows {
			node := step.ExtractionNode{
				NodeType: row.NodeType,
			}
			if row.Text.Valid {
				node.Text = row.Text.String
			}
			nodes = append(nodes, node)
		}
		extractionToChunk.Nodes = nodes

		return extractionToChunk, nil
	}
}
