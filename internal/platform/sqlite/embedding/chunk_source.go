package embedding

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"rag/internal/embedding/step"
)

type ChunksSource struct {
	dbClient  *sql.DB
	lastID    int64
	tableName string
	Logger    *slog.Logger
}

func NewChunkSource(db *sql.DB, tableName string, logger *slog.Logger) (ChunksSource, error) {
	source := ChunksSource{}
	source.dbClient = db
	source.lastID = 0
	source.tableName = tableName
	source.Logger = logger

	return source, nil
}

func (s *ChunksSource) NextChunks(limit int64, ctx context.Context) ([]step.ChunkToEmbed, error) {
	q := fmt.Sprintf(`SELECT c.id, c.text, c.document_id
FROM chunks c
WHERE c.id > ?
  AND NOT EXISTS (SELECT 1 FROM %s e WHERE e.chunk_id = c.id)
ORDER BY c.id
LIMIT ?`, s.tableName)

	rows, err := s.dbClient.QueryContext(ctx, q, s.lastID, limit)
	if err != nil {
		return []step.ChunkToEmbed{}, err
	}
	defer rows.Close()

	type chunkRow struct {
		ID    int64
		Text  string
		DocID int64
	}

	chunkRows := []chunkRow{}

	for rows.Next() {
		var row chunkRow
		if err := rows.Scan(&row.ID, &row.Text, &row.DocID); err != nil {
			return []step.ChunkToEmbed{}, err
		}
		chunkRows = append(chunkRows, row)
	}
	if err := rows.Err(); err != nil {
		return []step.ChunkToEmbed{}, err
	}

	if len(chunkRows) == 0 {
		return []step.ChunkToEmbed{}, nil
	}

	nextChunks := make([]step.ChunkToEmbed, 0, len(chunkRows))
	for _, row := range chunkRows {
		nextChunks = append(nextChunks, step.ChunkToEmbed{
			ChunkID: row.ID,
			Text:    row.Text,
			DocID:   row.DocID,
		})
	}

	s.lastID = chunkRows[len(chunkRows)-1].ID

	return nextChunks, nil
}
