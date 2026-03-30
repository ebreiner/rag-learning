package retrieve

import (
	"context"
	"database/sql"
	"fmt"
	"rag/internal/db/normalize"
	querries "rag/internal/db/retrieve"
	"rag/internal/embedding"
	"sort"
)

type QueryStrategy int

const (
	Embedding QueryStrategy = iota
	FTS
	Hybrid
)

type Query struct {
	Query      string
	normalized string
	embedding  []float32
	Strategy   QueryStrategy
	K          uint16
	ctx        context.Context
	db         *sql.DB
}

type QueryResult struct {
	Query   string
	Results querries.ChunkList
}

func NewQuery(ctx context.Context, db *sql.DB) Query {
	q := Query{}
	q.ctx = ctx
	q.db = db
	return q
}

func (q *Query) Run() (QueryResult, error) {
	result := QueryResult{}
	chunkIDs := make(querries.ChunkList)
	q.normalized = normalize.NormalizeText(q.Query)
	var queryErr error
	switch q.Strategy {
	case Embedding:
		chunkIDs, queryErr = q.byEmbedding()
	case FTS:
		chunkIDs, queryErr = q.byFTS()
	case Hybrid:
		chunkIDs, queryErr = q.byHybrid()
	default:
		queryErr = fmt.Errorf("query failed: missing or unknown query type '%d'", q.Strategy)
	}
	if queryErr != nil {
		return result, fmt.Errorf("query failed: %s", queryErr.Error())
	}
	chunks, err := querries.ChunksForIDs(q.ctx, q.db, chunkIDs)
	if err != nil {
		return result, fmt.Errorf("error retrieving chunks from db: %s", err.Error())
	}
	result.Query = q.normalized
	result.Results = chunks
	return result, nil
}

func (q *Query) byEmbedding() (querries.ChunkList, error) {
	chunks := make(querries.ChunkList)
	embedding, err := embedding.Embed(q.ctx, q.normalized)
	q.embedding = embedding.Vec
	if err != nil {
		return chunks, fmt.Errorf("error embedding querry: %s", err.Error())
	}
	chunks, err = querries.TopKByVec(q.db, 25, q.embedding, q.ctx)
	if err != nil {
		return chunks, fmt.Errorf("error running ann: %s", err.Error())
	} else {
		return chunks, nil
	}
}

func (q *Query) byFTS() (querries.ChunkList, error) {
	chunks, err := querries.TopKByFts(q.db, q.K, q.normalized, q.ctx)
	if err != nil {
		return chunks, fmt.Errorf("error running fts: %s", err.Error())
	} else {
		return chunks, nil
	}
}

func (q *Query) byHybrid() (querries.ChunkList, error) {
	chunks := make(querries.ChunkList)
	// Increase candidate pool for RRF with score from fts and distance from embedding
	var kRRF uint16
	kRRF = q.K * 4
	byEmbedding, err := q.byEmbedding()
	if err != nil {
		return chunks, fmt.Errorf("error running hybrid: %s", err.Error())
	}
	byFTS, err := q.byFTS()
	if err != nil {
		return chunks, fmt.Errorf("error running hybrid: %s", err.Error())
	}
	rankFusion := doRFF(byFTS, byEmbedding, kRRF)
	// Reduce back to k
	for rank := 1; rank < int(q.K); rank++ {
		pos := uint16(rank)
		chunks[pos] = rankFusion[pos]
	}

	return chunks, nil
}

func (q *Query) CutQueryString() string {
	return q.normalized[:50]
}

func doRFF(fts, ann querries.ChunkList, kRRF uint16) querries.ChunkList {
	type scoredChunk struct {
		chunk querries.Chunk
		score float64
	}
	scores := map[int64]*scoredChunk{}

	// Helper to add RRF score from one list
	addScores := func(list querries.ChunkList) {
		for rank, c := range list {
			if _, ok := scores[c.ID]; !ok {
				// copy struct to avoid overwriting original
				cc := c
				scores[c.ID] = &scoredChunk{
					chunk: cc,
					score: 0,
				}
			}
			scores[c.ID].score += 1.0 / float64(kRRF+rank)
		}
	}

	addScores(fts)
	addScores(ann)

	// Convert to slice for sorting
	resultSlice := make([]scoredChunk, 0, len(scores))
	for _, score := range scores {
		resultSlice = append(resultSlice, *score)
	}

	// Sort descending by RRF score
	sort.Slice(resultSlice, func(i, j int) bool {
		return resultSlice[i].score > resultSlice[j].score
	})

	// Define threshold for don't being to confident when we're not
	threshold := 1.0 / 60 / 5
	// Build output ChunkList, update rank
	output := make(querries.ChunkList, len(resultSlice))
	for i, chunk := range resultSlice {
		if chunk.score < threshold {
			break
		}
		rank := uint16(i + 1) // ranks start at 1
		chunk.chunk.Rank = rank
		chunk.chunk.Distance = chunk.score
		output[rank] = chunk.chunk
	}

	return output
}
