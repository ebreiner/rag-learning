package sqlite

import (
	"database/sql"
	"fmt"
	"rag/internal/platform/sqlite/querries"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func NewConn(dbPath string, rebuildFTSIndex bool) (*sql.DB, error) {
	sqlite_vec.Auto()
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("erro opening db connection: %w", err)
	}
	_, err = db.Exec(querries.Manual)
	if err != nil {
		return db, fmt.Errorf("error running init sql: %w", err)
	}
	_, err = db.Exec(querries.SQLC)
	if err != nil {
		return db, fmt.Errorf("error running init sql: %w", err)
	}
	_, err = db.Exec(querries.FTSTrigger)
	if err != nil {
		return db, fmt.Errorf("error running init sql: %w", err)
	}

	if rebuildFTSIndex {
		_, err = db.Exec("INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')")
		if err != nil {
			return db, fmt.Errorf("error running init sql: %w", err)
		}
	}

	db.SetMaxOpenConns(8)

	return db, nil
}
