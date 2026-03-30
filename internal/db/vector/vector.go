package vector

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/mattn/go-sqlite3"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed vector.so
var vectorSO []byte

func init() {
	fmt.Print("loading sqlite-vector extension..\n")
	tmpFile, err := os.CreateTemp("", "sqlite-vector-ext.so")
	if err != nil {
		log.Fatalf("error tmp file for vector-ext: %s", err.Error())
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(vectorSO); err != nil {
		tmpFile.Close()
		log.Fatalf("error writing vector-ext to tmp file: %s", err.Error())
	}

	if err := tmpFile.Close(); err != nil {
		log.Fatalf("error closing tmp file for vector-ext: %s", err.Error())
	}

	sql.Register("sqlite-vector", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			if err := conn.LoadExtension(tmpFile.Name(), "sqlite3_vector_init"); err != nil {
				return err
			}
			fmt.Printf("sqlite-vector extension successfully loaded from tmp file %s\n", tmpFile.Name())
			fmt.Println("check version of ext as test..")
			if _, err := conn.ExecContext(context.Background(), "SELECT vector_version();", nil); err != nil {
				return fmt.Errorf("version check against vector-ext failed: %s", err.Error())
			} else {
				fmt.Println("test successful")
				return nil
			}
		},
	})
}
