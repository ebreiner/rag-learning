package step

import (
	"context"
	"fmt"
	"math"
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
	defer stepSpan.End()

	stepSpan.SetAttributes(
		attribute.String("query.query", query),
		attribute.Int64("query.k", k),
		attribute.String("query.strategy", string(strategy)),
	)

	retrieveCtx, retrieveSpan := tracer.Start(stepCtx, "retrieval-run")
	defer retrieveSpan.End()
	var chunkIDs RetrievedChunkIDs
	var scoreByID map[int64]float64
	switch strategy {
	case FTS:
		ids, err := fts(retrieveCtx, query, k, retriever)
		if err != nil {
			return retrievedChunks, err
		}
		chunkIDs = ids

	case Embedding:
		ids, err := ann(retrieveCtx, query, k, embedClient, retriever)
		if err != nil {
			return retrievedChunks, err
		}
		chunkIDs = ids

	case Hybrid:
		scored, err := hybrid(retrieveCtx, query, k, retriever, embedClient)
		if err != nil {
			return retrievedChunks, err
		}
		chunkIDs = make(RetrievedChunkIDs, len(scored))
		scoreByID = make(map[int64]float64, len(scored))
		for i, s := range scored {
			chunkIDs[i] = s.ID
			scoreByID[s.ID] = math.Round(s.Score*1e4) / 1e4
		}

	default:
		return retrievedChunks, fmt.Errorf("unknown retrieval strategy: %s", strategy)
	}
	retrieveSpan.End()

	hydrateCtx, hydrateSpan := tracer.Start(stepCtx, "hydrate_chunks")
	defer hydrateSpan.End()
	hydratedChunks, err := hydrator.HydrateChunks(hydrateCtx, chunkIDs)
	if err != nil {
		return retrievedChunks, err
	}

	for i := range hydratedChunks {
		if score, ok := scoreByID[hydratedChunks[i].ID]; ok {
			hydratedChunks[i].Score = score
		}
	}

	hydrateSpan.End()
	stepSpan.End()
	return hydratedChunks, nil
}

func hybrid(ctx context.Context, query string, k int64, retriever TopKRetriever, embedClient EmbedClient) ([]scoredChunkID, error) {
	hybridK := k * 2

	ftsIDs, err := fts(ctx, query, hybridK, retriever)
	if err != nil {
		return nil, err
	}

	annIDs, err := ann(ctx, query, hybridK, embedClient, retriever)
	if err != nil {
		return nil, err
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

func rrfMerge(limit int64, rankings ...RetrievedChunkIDs) []scoredChunkID {
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

	return scored[:limit]
}
