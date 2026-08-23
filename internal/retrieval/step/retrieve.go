package step

import (
	"context"
	"fmt"
	"sort"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const rrfK int64 = 60

func RunRetrieval(
	ctx context.Context,
	query string,
	strategy RetrievalStrategy,
	k int64,
	hydrator ChunkHydrator,
	retriever TopKRetriever,
	embedClient EmbedClient,
) ([]RetrievedChunk, error) {
	retrievedChunks := make([]RetrievedChunk, 0)

	tracer := otel.Tracer("rag-cli-sdk")
	stepCtx, stepSpan := tracer.Start(ctx, "step-retrieval")

	stepSpan.SetAttributes(
		attribute.String("query.query", query),
		attribute.Int64("query.k", k),
		attribute.String("query.strategy", string(strategy)),
	)

	retrieveCtx, retrieveSpan := tracer.Start(stepCtx, "retrieval-run")
	var chunkIDs RetrievedChunkIDs
	switch strategy {
	case FTS:
		ids, err := fts(retrieveCtx, query, k, retriever)
		if err != nil {
			retrieveSpan.End()
			stepSpan.End()
			return retrievedChunks, err
		}
		chunkIDs = ids

	case Embedding:
		ids, err := ann(retrieveCtx, query, k, embedClient, retriever)
		if err != nil {
			retrieveSpan.End()
			stepSpan.End()
			return retrievedChunks, err
		}
		chunkIDs = ids

	case Hybrid:
		ids, err := hybrid(retrieveCtx, query, k, retriever, embedClient)
		if err != nil {
			retrieveSpan.End()
			stepSpan.End()
			return retrievedChunks, err
		}
		chunkIDs = ids

	default:
		retrieveSpan.End()
		stepSpan.End()
		return retrievedChunks, fmt.Errorf("unknown retrieval strategy: %s", strategy)
	}
	retrieveSpan.End()

	hydrateCtx, hydrateSpan := tracer.Start(stepCtx, "hydrate_chunks")
	hydratedChunks, err := hydrator.HydrateChunks(hydrateCtx, chunkIDs)
	if err != nil {
		hydrateSpan.End()
		stepSpan.End()
		return retrievedChunks, err
	}

	hydrateSpan.End()
	stepSpan.End()
	return hydratedChunks, nil
}

func hybrid(ctx context.Context, query string, k int64, retriever TopKRetriever, embedClient EmbedClient) (RetrievedChunkIDs, error) {
	var chunkIDs RetrievedChunkIDs

	hybridK := k * 2

	ftsIDs, err := fts(ctx, query, hybridK, retriever)
	if err != nil {
		return chunkIDs, err
	}

	annIDs, err := ann(ctx, query, hybridK, embedClient, retriever)
	if err != nil {
		return chunkIDs, err
	}

	return rrfMerge(k, ftsIDs, annIDs), nil
}

func fts(ctx context.Context, query string, k int64, retriever TopKRetriever) (RetrievedChunkIDs, error) {
	chunkIDs, err := retriever.TopKByFTS(ctx, query, k)
	if err != nil {
		return chunkIDs, err
	}

	return chunkIDs, nil
}

func ann(ctx context.Context, query string, k int64, embedClient EmbedClient, retriever TopKRetriever) (RetrievedChunkIDs, error) {
	var chunkIDs RetrievedChunkIDs

	embeddedQ, err := embedClient.EmbedQuery(ctx, query)
	if err != nil {
		return chunkIDs, err
	}

	chunkIDs, err = retriever.TopKByANN(ctx, embeddedQ, k)
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
