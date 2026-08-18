package sqlite

import (
	"database/sql"
	"fmt"
	"rag/internal/platform/sqlite/querries"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func NewConn(dbPath string) (*sql.DB, error) {
	sqlite_vec.Auto()
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL", dbPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("erro opening db connection: %s", err.Error())
	} else {
	}
	_, err = db.Exec(querries.Manual)
	if err != nil {
		return db, fmt.Errorf("error running init sql: %s\n", err.Error())
	}
	_, err = db.Exec(querries.SQLC)
	if err != nil {
		return db, fmt.Errorf("error running init sql: %s\n", err.Error())
	}
	_, err = db.Exec(querries.FTSTrigger)
	if err != nil {
		return db, fmt.Errorf("error running init sql: %s\n", err.Error())
	}
	_, err = db.Exec("INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')")
	if err != nil {
		return db, fmt.Errorf("error running init sql: %s\n", err.Error())
	}

	db.SetMaxOpenConns(8)

	return db, nil
}
