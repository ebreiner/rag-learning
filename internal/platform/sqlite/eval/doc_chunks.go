package eval

import (
	"context"
	"database/sql"
	"log/slog"
	"rag/internal/eval/step"
	"rag/internal/platform/sqlite/querries"
)

type DocChunksProvider struct {
	q      *querries.Queries
	logger *slog.Logger
}

func NewDocChunksProvider(db *sql.DB, logger *slog.Logger) DocChunksProvider {
	return DocChunksProvider{q: querries.New(db), logger: logger}
}

func (p *DocChunksProvider) ChunksOfDoc(ctx context.Context, docName string) ([]step.Chunk, bool, error) {
	n, err := p.q.EvalDocExists(ctx, docName)
	if err != nil {
		return nil, false, err
	}
	if n == 0 {
		return nil, false, nil
	}

	rows, err := p.q.EvalChunksOfDoc(ctx, docName)
	if err != nil {
		return nil, false, err
	}

	chunks := make([]step.Chunk, 0, len(rows))
	for _, row := range rows {
		chunks = append(chunks, step.Chunk{
			ID:         row.ID,
			DocName:    row.Name,
			Breadcrumb: row.Breadcrumb,
			Text:       row.Text,
		})
	}
	return chunks, true, nil
}
