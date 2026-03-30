package insert

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"rag/internal/db/insert/query"
	"rag/internal/db/schema"
	"rag/internal/db/utils"
	"sync"

	kreuzberg "github.com/kreuzberg-dev/kreuzberg/packages/go/v4"
)

type ChunkedDoc struct {
	Title  string
	Chunks []kreuzberg.Chunk
}

type InsertError struct {
	DocTitle   string
	ChunkIndex int64
	ChunkID    int64
	Err        string
}

func (e *InsertError) Error() string {
	return e.Err
}

// doneWG works on count of docs + 1 to signal when writing is done
func HandleChunks(ctx context.Context, db *sql.DB, initWG *sync.WaitGroup, doneWG *sync.WaitGroup, writer chan ChunkedDoc, errChan chan InsertError) {
	defer db.Close()
	initDB(db)
	if err := tuneConn(db); err != nil {
		log.Fatalf(fmt.Sprintf("error tuning ingest perf: %s", err.Error()))
	}
	var lastID int64
	err := db.QueryRow("SELECT COALESCE(MAX(id), 0) FROM chunks;").Scan(&lastID)
	fmt.Printf("last id: %d\n", lastID)
	if err != nil {
		log.Fatalf("error fetching last row id: %s", err.Error())
	}
	tx, err := db.BeginTx(ctx, nil)
	q := query.New(tx)
	initWG.Done()
	for doc := range writer {
		fmt.Printf("writer processing doc: %s\n", doc.Title)
		docID, err := q.CreateDocument(ctx, doc.Title)
		if err != nil {
			errChan <- InsertError{
				DocTitle:   doc.Title,
				ChunkIndex: 0,
				ChunkID:    0,
				Err:        fmt.Sprintf("error creating doc %s: %s", doc.Title, err.Error()),
			}
			doneWG.Done()
			continue
		}

		for _, chunk := range doc.Chunks {
			lastID = lastID + 1
			param := query.InsertChunkParams{
				ID:         lastID,
				DocumentID: docID,
				ChunkIndex: int64(chunk.Metadata.ChunkIndex),
				ChunkCount: int64(chunk.Metadata.TotalChunks),
				Text:       chunk.Content,
			}
			if err := q.InsertChunk(ctx, param); err != nil {
				errChan <- InsertError{
					DocTitle:   doc.Title,
					ChunkIndex: int64(chunk.Metadata.ChunkIndex),
					ChunkID:    lastID,
					Err:        fmt.Sprintf("error insert chunk %d: %s", lastID, err.Error()),
				}
				doneWG.Done()
				continue
			}
			packed, err := utils.PackEmbedding(chunk.Embedding)
			if err != nil {
				errChan <- InsertError{
					DocTitle:   doc.Title,
					ChunkIndex: int64(chunk.Metadata.ChunkIndex),
					ChunkID:    lastID,
					Err:        fmt.Sprintf("error packing embedding for insert for chunk %d: %s", lastID, err.Error()),
				}
				doneWG.Done()
				continue
			}
			_, err = tx.Exec("INSERT INTO embeddings_balanced_768(chunk_id, embedding) VALUES (?,?)", lastID, packed.Bytes())
			if err != nil {
				errChan <- InsertError{
					DocTitle:   doc.Title,
					ChunkIndex: int64(chunk.Metadata.ChunkIndex),
					ChunkID:    lastID,
					Err:        fmt.Sprintf("error insert embedding for chunk %d: %s", lastID, err.Error()),
				}
				doneWG.Done()
				continue
			}

		}
		fmt.Printf("writer finished processing doc: %s\n", doc.Title)
		doneWG.Done()
	}
	fmt.Println("finished writing data, committing transaction..")
	if err := tx.Commit(); err != nil {
		errChan <- InsertError{
			DocTitle: "commit_tx_error",
			Err:      fmt.Sprintf("error committing transaction: %s", err.Error()),
		}
		doneWG.Done()
		return
	}

	fmt.Println("successfully committed transaction")
	fmt.Println("rebuilding fts index..")
	if err := rebuildFTSIndex(db); err != nil {
		errChan <- InsertError{
			DocTitle: "rebuild_fts_index",
			Err:      fmt.Sprintf("error rebuilding fts index: %s", err.Error()),
		}
		doneWG.Done()
		return
	}

	doneWG.Done()
	return
}

func initDB(db *sql.DB) {
	fmt.Print("setting up database tables..\n")
	_, err := db.Exec(schema.Manual)
	if err != nil {
		log.Fatalf("error running init sql: %s\n", err.Error())
	}
	_, err = db.Exec(schema.SQLC)
	if err != nil {
		log.Fatalf("error running init sql: %s\n", err.Error())
	}
	fmt.Println("setting up fts trigger..")
	_, err = db.Exec(schema.FTSTrigger)
	if err != nil {
		log.Fatalf("error running init sql: %s\n", err.Error())
	}
	fmt.Println("setup of db done.")
}

func rebuildFTSIndex(db *sql.DB) error {
	_, err := db.Exec("INSERT INTO chunks_fts(chunks_fts) VALUES('rebuild')")
	if err != nil {
		return err
	} else {
		return nil
	}
}

func tuneConn(db *sql.DB) error {
	if _, err := db.Exec("PRAGMA temp_store=MEMORY"); err != nil {
		return fmt.Errorf("error setting PRAGMA: %s", err.Error())
	}

	return nil
}
