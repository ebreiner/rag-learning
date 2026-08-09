package step

import (
	"fmt"
	"sort"
)

const rrfK int64 = 60

func RunRetrieval(
	query string,
	strategy RetrievalStrategy,
	k int64,
	hydrator ChunkHydrator,
	retriever TopKRetriever,
	embedClient EmbedClient,
) ([]RetrievedChunk, error) {
	retrievedChunks := make([]RetrievedChunk, 0)

	var chunkIDs RetrievedChunkIDs
	switch strategy {
	case FTS:
		ids, err := fts(query, k, retriever)
		if err != nil {
			return retrievedChunks, err
		}
		chunkIDs = ids

	case Embedding:
		ids, err := ann(query, k, embedClient, retriever)
		if err != nil {
			return retrievedChunks, err
		}
		chunkIDs = ids

	case Hybrid:
		ids, err := hybrid(query, k, retriever, embedClient)
		if err != nil {
			return retrievedChunks, err
		}
		chunkIDs = ids

	default:
		return retrievedChunks, fmt.Errorf("unknown retrieval strategy: %d", strategy)
	}

	hydratedChunks, err := hydrator.HydrateChunks(chunkIDs)
	if err != nil {
		return retrievedChunks, err
	}

	return hydratedChunks, nil
}

func hybrid(query string, k int64, retriever TopKRetriever, embedClient EmbedClient) (RetrievedChunkIDs, error) {
	var chunkIDs RetrievedChunkIDs

	hybridK := k * 4

	ftsIDs, err := fts(query, hybridK, retriever)
	if err != nil {
		return chunkIDs, err
	}

	annIDs, err := ann(query, hybridK, embedClient, retriever)
	if err != nil {
		return chunkIDs, err
	}

	return rrfMerge(k, ftsIDs, annIDs), nil
}

func fts(query string, k int64, retriever TopKRetriever) (RetrievedChunkIDs, error) {
	chunkIDs, err := retriever.TopKByFTS(query, k)
	if err != nil {
		return chunkIDs, err
	}

	return chunkIDs, nil
}

func ann(query string, k int64, embedClient EmbedClient, retriever TopKRetriever) (RetrievedChunkIDs, error) {
	var chunkIDs RetrievedChunkIDs

	embeddedQ, err := embedClient.EmbedQuery(query)
	if err != nil {
		return chunkIDs, err
	}

	chunkIDs, err = retriever.TopKByANN(embeddedQ, k)
	if err != nil {
		return chunkIDs, err
	}

	return chunkIDs, nil
}

type scoredChunkID struct {
	ID    int64
	Score float64
}

func rrfMerge(limit int64, rankings ...RetrievedChunkIDs) RetrievedChunkIDs {
	scores := make(map[int64]float64)

	for _, ranking := range rankings {
		for idx, chunkID := range ranking {
			rank := int64(idx) + 1
			scores[chunkID] += 1.0 / float64(rrfK+rank)
		}
	}

	scored := make([]scoredChunkID, 0, len(scores))
	for id, score := range scores {
		scored = append(scored, scoredChunkID{
			ID:    id,
			Score: score,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score == scored[j].Score {
			return scored[i].ID < scored[j].ID
		}
		return scored[i].Score > scored[j].Score
	})

	if limit > int64(len(scored)) {
		limit = int64(len(scored))
	}

	result := make(RetrievedChunkIDs, 0, limit)
	for i := int64(0); i < limit; i++ {
		result = append(result, scored[i].ID)
	}

	return result
}
