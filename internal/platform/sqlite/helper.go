package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"rag/internal/platform/sqlite/querries"
	"strings"
)

func PackVector(vec []float64) ([]byte, error) {
	narrowed := make([]float32, len(vec))
	for i, v := range vec {
		narrowed[i] = float32(v)
	}
	buf := new(bytes.Buffer)
	if err := binary.Write(buf, binary.LittleEndian, narrowed); err != nil {
		return nil, fmt.Errorf("packing vector: %w", err)
	}
	return buf.Bytes(), nil
}

func normalizeModel(model string) string {
	model = strings.ReplaceAll(model, ":", "_")
	return strings.ReplaceAll(model, "-", "_")
}

func vecTableName(dim int64, model string) (string, error) {
	if dim == 0 || model == "" {
		return "", fmt.Errorf("table setup failed, dim or model missing: %d   %s", dim, model)
	}
	return fmt.Sprintf("embeddings_%s_%d", normalizeModel(model), dim), nil
}

// model comes from a local cli flag, not via remote serve stuff, already has direct fs access to sqlite.
// no privilege boundary for injection here, but keep an eye out for future changes, this can get dangerous really
// sneaky.
func SetupVecTable(ctx context.Context, client *sql.DB, dim int64, model string) (tableName string, err error) {
	table, err := vecTableName(dim, model)
	if err != nil {
		return "", err
	}
	createStmt := fmt.Sprintf(
		`CREATE VIRTUAL TABLE IF NOT EXISTS "%s" USING vec0(chunk_id INTEGER PRIMARY KEY, embedding float[%d])`,
		table, dim,
	)
	if _, err := client.ExecContext(ctx, createStmt); err != nil {
		return "", fmt.Errorf("creating embedding table %q: %w", table, err)
	}

	return table, nil
}

type VecTableNotExistErr struct {
	err string
}

func (e VecTableNotExistErr) Error() string {
	return e.err
}

// returns the table name for this model+dim only if it already exists
func LookupVecTable(ctx context.Context, client *sql.DB, dim int64, model string) (tableName string, err error) {
	table, err := vecTableName(dim, model)
	if err != nil {
		return "", err
	}

	var found string
	row := client.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
	)
	if err := row.Scan(&found); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", VecTableNotExistErr{err: fmt.Sprintf("table %q does not exist", table)}
		}
		return "", err
	}

	return found, nil
}

func FlushAllEmbeddings(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name LIKE 'embeddings_%'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return err
		}
		tables = append(tables, name)
	}

	for _, t := range tables {
		if _, err := db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %q", t)); err != nil {
			return fmt.Errorf("flushing %s: %w", t, err)
		}
	}
	return nil
}

func FlushChunkTable(ctx context.Context, db *sql.DB) error {
	q := querries.New(db)
	if err := q.FlushChunks(ctx); err != nil {
		return err
	}
	return nil
}
