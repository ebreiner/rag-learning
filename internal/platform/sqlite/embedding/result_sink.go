package embedding

import (
	"context"
	"database/sql"
	"rag/internal/embedding/step"
	"rag/internal/platform/sqlite"
)

type ResultSink struct {
	dbClient *sql.DB
	ctx      context.Context
}

func NewEmbeddingsResultSink(db *sql.DB, ctx context.Context) (ResultSink, error) {
	sink := ResultSink{}
	sink.dbClient = db
	sink.ctx = ctx

	return sink, nil
}

func (s ResultSink) SaveEmbeddings(embeddings []step.EmbeddingToSave) error {
	tx, err := s.dbClient.BeginTx(s.ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, embedding := range embeddings {
		packed, err := sqlite.PackVector(embedding.Vector)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(s.ctx, "INSERT INTO embeddings_balanced_1536(chunk_id, embedding) VALUES (?,?)", embedding.ChunkID, packed)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(s.ctx, "UPDATE chunks SET embedded = 1 WHERE id =?", embedding.ChunkID)
		if err != nil {
			return err
		}
	}

	err = tx.Commit()
	if err != nil {
		return err
	} else {
		return nil
	}
}
