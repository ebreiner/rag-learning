package retrieval

import (
	"context"
	"database/sql"
	"rag/internal/platform/sqlite"
)

type SQLiteRetriever struct {
	ctx context.Context
	db  *sql.DB
}

func NewSQLiteRetriever(ctx context.Context) (SQLiteRetriever, error) {
	retriever := SQLiteRetriever{ctx: ctx}

	db, err := sqlite.NewConn()
	if err != nil {
		return retriever, err
	}
	retriever.db = db

	return retriever, nil
}
