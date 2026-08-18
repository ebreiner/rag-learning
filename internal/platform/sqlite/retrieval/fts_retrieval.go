package retrieval

import (
	"context"
	"fmt"
	"rag/internal/retrieval/step"
	"strings"
)

func (r *SQLiteRetriever) TopKByFTS(query string, k int64, ctx context.Context) (step.RetrievedChunkIDs, error) {
	chunkIDs := make([]int64, 0)

	parts := strings.Fields(query)
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		if len([]rune(part)) > 2 {
			terms = append(terms, part)
		} else {
			// spaces werden automatisch als AND interpretiert von fts. daher filtern wir relevante (wörter länger 2) raus und
			// bauen eine liste. diese werden dann via OR verkettet.
			continue
		}
	}
	// Schadet nicht und kann Präzison erhöhen
	terms = append(terms, query)
	matchTerm := ""
	quote := func(s string) string {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	for idx, el := range terms {
		if idx == len(terms)-1 {
			matchTerm = matchTerm + quote(el)
			continue
		}
		if idx == 0 {
			matchTerm = quote(el) + " OR "
			continue
		}

		matchTerm = matchTerm + quote(el) + " OR "
	}

	q := `
	SELECT rowid, bm25(chunks_fts)
	FROM chunks_fts
	WHERE chunks_fts MATCH ?
	ORDER BY bm25(chunks_fts)
	LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, q, matchTerm, k)
	if err != nil {
		return chunkIDs, fmt.Errorf("error querring rows: %s", err.Error())
	}
	defer rows.Close()

	for rows.Next() {
		if err := rows.Err(); err != nil {
			return chunkIDs, fmt.Errorf("error scanning rows for fts: %s", err.Error())
		}
		var id int64
		var score float64
		if err := rows.Scan(&id, &score); err != nil {
			return chunkIDs, fmt.Errorf("error scanning top k row result: %s", err.Error())
		}
		chunkIDs = append(chunkIDs, id)
	}

	return chunkIDs, nil
}
