package extraction

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

func (e *ExtractedDocSink) SaveExtractedDoc(doc step.ExtractedDoc) error {
	tx, err := e.dbClient.BeginTx(e.ctx, nil)
	if err != nil {
		return err
	}
	q := querries.New(tx)
	defer tx.Rollback()

	_, err = q.ExistsDocument(e.ctx, doc.Source.SHA256)
	if err == nil {
		log.Printf("warning: duplicate document, skipping %s\n", doc.Source.Name)
		return nil
	} else if errors.Is(err, sql.ErrNoRows) {

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

		buf := new(bytes.Buffer)
		if err := binary.Write(buf, binary.LittleEndian, doc.Metadata.QualityScore); err != nil {
			txErr := tx.Rollback()
			if txErr != nil {
				return fmt.Errorf("error rolling back transaction at buffer write: %s\noriginal error: %s", txErr, err)
			}

			return fmt.Errorf("error converting embedding to buffer: %s", err.Error())
		}

		extractionParam := querries.CreateExtractionParams{
			RepresentationID: reprID,
			MimeType:         doc.Metadata.MimeType,
			QualityScore:     buf.Bytes(),
			CreatedAt:        time.Now(),
			MetadataJson:     sql.NullString{String: string(doc.Metadata.Additional)},
		}
		if len(doc.Metadata.Additional) > 0 {
			extractionParam.MetadataJson.Valid = true
		}

		extID, err := q.CreateExtraction(e.ctx, extractionParam)
		if err != nil {
			txErr := tx.Rollback()
			if txErr != nil {
				return fmt.Errorf("error rolling back transaction at extraction creation: %s\noriginal error: %s", txErr, err)
			}
			return err
		}
		for _, node := range doc.Nodes {
			childrenJson := sql.NullString{}
			var childrenIndexes struct {
				ChildrenIndexes *[]int64 `json:"children_indexes"`
			}
			if node.ChildrenIndexes != nil {
				childrenIndexes.ChildrenIndexes = node.ChildrenIndexes
				marshal, err := json.Marshal(childrenIndexes)

				if err != nil {
					txErr := tx.Rollback()
					if txErr != nil {
						return fmt.Errorf("error rolling back transaction at marshaling children indexes: %s\noriginal error: %s", txErr, err)
					}
					return err
				}

				childrenJson.String = string(marshal)
				childrenJson.Valid = true
			}

			param := querries.CreateExtractionNodeParams{
				ExtractionID:        extID,
				CreatedAt:           time.Now(),
				NodeID:              node.ID,
				NodeType:            node.NodeType,
				ChildrenIndexesJson: childrenJson,
			}

			if node.ParentIndex != nil {
				param.ParentIndex = sql.NullInt64{Int64: *node.ParentIndex, Valid: true}
			}

			if node.Level != nil {
				param.Level = sql.NullInt64{Int64: *node.Level, Valid: true}
			}

			if node.Text != nil {
				param.Text = sql.NullString{String: *node.Text, Valid: true}
			}

			err = q.CreateExtractionNode(e.ctx, param)
			if err != nil {
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

	} else {
		return err
	}

	return nil
}
