package retrieval

import (
	"context"
	"database/sql"
)

type SQLiteRetriever struct {
	ctx context.Context
	db  *sql.DB
}

func NewSQLiteRetriever(db *sql.DB, ctx context.Context) (SQLiteRetriever, error) {
	retriever := SQLiteRetriever{ctx: ctx}
	retriever.db = db
	return retriever, nil
}
