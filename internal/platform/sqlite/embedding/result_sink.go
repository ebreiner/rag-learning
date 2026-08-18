package embedding

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"rag/internal/embedding/step"
	"rag/internal/platform/sqlite"
)

type ResultSink struct {
	dbClient *sql.DB
	Logger   *slog.Logger
}

func NewEmbeddingsResultSink(db *sql.DB, logger *slog.Logger) (ResultSink, error) {
	sink := ResultSink{}
	sink.dbClient = db
	sink.Logger = logger

	return sink, nil
}

func (s ResultSink) SaveEmbeddings(embeddings step.EmbeddingsToSave, ctx context.Context) error {
	tableName, err := sqlite.SetupVecTable(s.dbClient, ctx, embeddings.Dim, embeddings.Model)
	if err != nil {
		return err
	}

	tx, err := s.dbClient.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	insertQuerry := fmt.Sprintf("INSERT INTO %s(chunk_id, embedding) VALUES (?,?)", tableName)
	for _, embedding := range embeddings.Embeddings {
		packed, err := sqlite.PackVector(embedding.Vector)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, insertQuerry, embedding.ChunkID, packed)
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
