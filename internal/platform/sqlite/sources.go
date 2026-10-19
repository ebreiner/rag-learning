package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"rag/internal/platform/config"
	"rag/internal/platform/sqlite/querries"
)

func SaveSources(ctx context.Context, db *sql.DB, sources []config.Source) (err error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, tx.Rollback())
		}
	}()

	q := querries.New(tx)
	for _, src := range sources {
		param := querries.SaveSourceParams{Name: src.Name, Weight: src.Weight}
		if err := q.SaveSource(ctx, param); err != nil {
			return fmt.Errorf("saving source %q: %w", src.Name, err)
		}
	}

	return tx.Commit()
}
