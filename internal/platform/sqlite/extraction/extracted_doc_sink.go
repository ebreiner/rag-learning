package extraction

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"rag/internal/extract/step"
	"rag/internal/platform/sqlite/querries"
	"time"
)

type ExtractedDocSink struct {
	dbClient *sql.DB
	ctx      context.Context
}

func NewExtractedDocSink(db *sql.DB, ctx context.Context) (ExtractedDocSink, error) {
	sink := ExtractedDocSink{}
	sink.dbClient = db
	sink.ctx = ctx

	return sink, nil
}

func (e *ExtractedDocSink) ExistsDoc(sha256 string) (bool, error) {
	q := querries.New(e.dbClient)
	_, err := q.ExistsDocument(e.ctx, sha256)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		} else {
			return false, err
		}
	} else {
		return true, nil
	}

}

func (e *ExtractedDocSink) SaveExtractedDoc(doc step.ExtractedDoc) error {
	tx, err := e.dbClient.BeginTx(e.ctx, nil)
	if err != nil {
		return err
	}
	q := querries.New(tx)
	defer tx.Rollback()

	docParam := querries.CreateDocumentParams{
		CreatedAt: time.Now(),
		Name:      doc.Source.Name,
		Sha256:    doc.Source.SHA256,
	}
	if len(doc.Source.Additional) > 0 {
		docAdditotionalMetadata, err := json.Marshal(doc.Source.Additional)
		if err != nil {
			txErr := tx.Rollback()
			if txErr != nil {
				return fmt.Errorf("error rolling back transaction: %s\noriginal error: %s", txErr, err)
			}
			return err
		}
		metadataString := string(docAdditotionalMetadata)

		docParam.MetadataJson = sql.NullString{String: metadataString, Valid: true}
	}

	docID, err := q.CreateDocument(e.ctx, docParam)
	if err != nil {
		txErr := tx.Rollback()
		if txErr != nil {
			return fmt.Errorf("error rolling back transaction at doc creation: %s\noriginal error: %s", txErr, err)
		}
		return err
	}
	representationParam := querries.CreateRepresentationParams{
		DocumentID: docID,
		Stage:      "extract",
		CreatedAt:  time.Now(),
	}

	reprID, err := q.CreateRepresentation(e.ctx, representationParam)
	if err != nil {
		txErr := tx.Rollback()
		if txErr != nil {
			return fmt.Errorf("error rolling back transaction at representation creation: %s\noriginal error: %s", txErr, err)
		}
		return err
	}

	extractionParam := querries.CreateExtractionParams{
		RepresentationID: reprID,
		CreatedAt:        time.Now(),
		MimeType:         doc.MimeType,
	}

	extID, err := q.CreateExtraction(e.ctx, extractionParam)
	if err != nil {
		txErr := tx.Rollback()
		if txErr != nil {
			return fmt.Errorf("error rolling back transaction at extraction creation: %s\noriginal error: %s", txErr, err)
		}
		return err
	}
	var walk func(node *step.Node, parentID string) error
	walk = func(node *step.Node, parentID string) error {
		contentJSON, err := marshalContent(node)
		if err != nil {
			return fmt.Errorf("marshaling content for node %s: %w", node.ID, err)
		}
		provJSON, err := json.Marshal(node.Provenance)
		if err != nil {
			return fmt.Errorf("marshaling provenance for node %s: %w", node.ID, err)
		}

		param := querries.CreateExtractionNodeParams{
			ExtractionID: extID,
			CreatedAt:    time.Now(),
			NodeID:       node.ID,
			Kind:         string(node.Kind),
			Layer:        string(node.Layer),
		}
		if parentID != "" {
			param.ParentID = sql.NullString{String: parentID, Valid: true}
		}
		if contentJSON != nil {
			param.ContentJson = sql.NullString{String: string(contentJSON), Valid: true}
		}
		if len(node.Provenance) > 0 {
			param.ProvenanceJson = sql.NullString{String: string(provJSON), Valid: true}
		}

		if err := q.CreateExtractionNode(e.ctx, param); err != nil {
			return err
		}

		for _, child := range node.Children {
			if err := walk(child, node.ID); err != nil {
				return err
			}
		}
		return nil
	}

	for _, root := range doc.RootNodes {
		if err := walk(root, ""); err != nil {
			txErr := tx.Rollback()
			if txErr != nil {
				return fmt.Errorf("error rolling back transaction at node insertion: %s\noriginal error: %s", txErr, err)
			}
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		txErr := tx.Rollback()
		if txErr != nil {
			return fmt.Errorf("error rolling back transaction at comitting: %s\noriginal error: %s", txErr, err)
		}
		return err
	}

	return nil
}

func marshalContent(node *step.Node) ([]byte, error) {
	switch node.Kind {
	case step.KindHeading:
		return json.Marshal(node.Heading)
	case step.KindParagraph:
		return json.Marshal(node.Paragraph)
	case step.KindCaption:
		return json.Marshal(node.Caption)
	case step.KindFootnote:
		return json.Marshal(node.Footnote)
	case step.KindListItem:
		return json.Marshal(node.ListItem)
	case step.KindTable:
		return json.Marshal(node.Table)
	case step.KindPicture:
		return json.Marshal(node.Picture)
	default:
		return nil, nil
	}
}
