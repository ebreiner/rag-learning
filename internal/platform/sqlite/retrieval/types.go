package retrieval

import (
	"database/sql"
	"log/slog"
)

type SQLiteRetriever struct {
	db     *sql.DB
	Logger *slog.Logger
}

func NewSQLiteRetriever(db *sql.DB, logger *slog.Logger) (*SQLiteRetriever, error) {
	retriever := SQLiteRetriever{}
	retriever.db = db
	retriever.Logger = logger
	return &retriever, nil
}
