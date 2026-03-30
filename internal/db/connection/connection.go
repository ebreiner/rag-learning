package connection

import (
	"database/sql"
	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
)

func NewConn() (*sql.DB, error) {
	sqlite_vec.Auto()
	db, err := sql.Open("sqlite3", "file:./data/data.db")
	if err != nil {
		return nil, err
	} else {
		return db, nil
	}
}
