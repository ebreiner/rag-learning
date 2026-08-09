package embedding

import (
	"context"
	"database/sql"
	"fmt"
	"rag/internal/embedding/step"
)

type ChunksSource struct {
	dbClient  *sql.DB
	ctx       context.Context
	lastID    int64
	tableName string
}

func NewChunkSource(db *sql.DB, ctx context.Context, tableName string) (ChunksSource, error) {
	source := ChunksSource{}
	source.dbClient = db
	source.ctx = ctx
	source.lastID = 0
	source.tableName = tableName

	return source, nil
}

func (s *ChunksSource) NextChunks(limit int64) ([]step.ChunkToEmbed, error) {
	q := fmt.Sprintf(`SELECT c.id, c.text
FROM chunks c
WHERE c.id > ?
  AND NOT EXISTS (SELECT 1 FROM %s e WHERE e.chunk_id = c.id)
ORDER BY c.id
LIMIT ?`, s.tableName)

	rows, err := s.dbClient.QueryContext(s.ctx, q, s.lastID, limit)
	if err != nil {
		return []step.ChunkToEmbed{}, err
	}
	defer rows.Close()

	type chunkRow struct {
		ID   int64
		Text string
	}

	chunkRows := []chunkRow{}

	for rows.Next() {
		var row chunkRow
		if err := rows.Scan(&row.ID, &row.Text); err != nil {
			return []step.ChunkToEmbed{}, err
		}
		chunkRows = append(chunkRows, row)
	}
	if err := rows.Err(); err != nil {
		return []step.ChunkToEmbed{}, err
	}

	if len(chunkRows) == 0 {
		fmt.Println("no rows returned")
		return nil, nil
	}

	nextChunks := make([]step.ChunkToEmbed, 0, len(chunkRows))
	for _, row := range chunkRows {
		nextChunks = append(nextChunks, step.ChunkToEmbed{
			ChunkID: row.ID,
			Text:    row.Text,
		})
	}

	s.lastID = chunkRows[len(chunkRows)-1].ID

	return nextChunks, nil
}
