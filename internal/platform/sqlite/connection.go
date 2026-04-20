package sqlite

import (
	"database/sql"
	"fmt"
	"rag/internal/platform/sqlite/querries"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func NewConn() (*sql.DB, error) {
	sqlite_vec.Auto()
	db, err := sql.Open("sqlite3", "file:./data/data.db")
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

	return db, nil
}
