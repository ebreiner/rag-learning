package step

import (
	"context"
	"fmt"
	"sort"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

const rrfK int64 = 60

func RunRetrieval(ctx context.Context, query string, k int64, strategy RetrievalStrategy, deps RetrievalDeps) ([]RetrievedChunk, error) {
	tracer := otel.Tracer("rag-cli-sdk")
	stepCtx, stepSpan := tracer.Start(ctx, "step-retrieval")
	defer stepSpan.End()

	stepSpan.SetAttributes(
		attribute.String("query.query", query),
		attribute.Int64("query.k", k),
		attribute.String("query.strategy", string(strategy)),
	)

	retrieveCtx, retrieveSpan := tracer.Start(stepCtx, "run-query")
	defer retrieveSpan.End()

	var err error
	var chunkIDs []ScoredChunkID
	var chunks []RetrievedChunk
	switch strategy {
	case StrategyANN:
		chunkIDs, err = runANN(retrieveCtx, query, k, deps.EmbeddingsClient, deps.Retriever)
	case StrategyFTS:
		chunkIDs, err = runFTS(retrieveCtx, query, k, deps.Retriever)
	case StrategyHybrid:
		chunkIDs, err = runHybrid(retrieveCtx, query, max(k, deps.PoolSize), deps.Fusion, deps.EmbeddingsClient, deps.Retriever)
	default:
		return nil, fmt.Errorf("unknown strategy: '%s'", strategy)
	}
	if err != nil {
		return nil, err
	}

	retrieveSpan.End()

	hydrateCtx, hydrateSpan := tracer.Start(stepCtx, "hydrate-chunks")
	defer hydrateSpan.End()
	hydrated, err := deps.Hydrator.HydrateChunks(hydrateCtx, chunkIDs)
	if err != nil {
		return nil, err
	}
	hydrateSpan.End()

	// phase 2 of hybrid: order the hydrated pool, then cut to k
	if strategy == StrategyHybrid {
		_, weightSpan := tracer.Start(stepCtx, "source-weight")
		hydrated = sourceRerank(hydrated)
		if len(hydrated) > int(k) {
			hydrated = hydrated[:k]
		}
		weightSpan.End()
	}

	renderCtx, renderSpan := tracer.Start(stepCtx, "render-chunks")
	defer renderSpan.End()
	rendered, err := deps.Renderer.RenderChunks(renderCtx, hydrated)
	if err != nil {
		return nil, err
	}
	chunks = rendered
	renderSpan.End()

	return chunks, nil
}

func runANN(ctx context.Context, query string, k int64, embed EmbedClient, retriever TopKRetriever) ([]ScoredChunkID, error) {
	embeddedQ, err := embed.EmbedQuery(ctx, query)
	if err != nil {
		return nil, err
	}

	chunkIDs, err := retriever.TopKByANN(ctx, embeddedQ, k)
	if err != nil {
		return chunkIDs, err
	}

	rank := int64(0)
	for i := range chunkIDs {
		rank++
		chunkIDs[i].Rank = rank
	}

	return chunkIDs, nil
}

func runFTS(ctx context.Context, query string, k int64, retriever TopKRetriever) ([]ScoredChunkID, error) {
	chunkIDs, err := retriever.TopKByFTS(ctx, query, k)
	if err != nil {
		return nil, err
	}
	rank := int64(0)
	for i := range chunkIDs {
		rank++
		chunkIDs[i].Rank = rank
	}

	return chunkIDs, nil
}

func runHybrid(ctx context.Context, query string, pool int64, fusion FusionConfig, embed EmbedClient, retriever TopKRetriever) ([]ScoredChunkID, error) {
	ftsIDs, err := runFTS(ctx, query, pool, retriever)
	if err != nil {
		return nil, err
	}

	annIDs, err := runANN(ctx, query, pool, embed, retriever)
	if err != nil {
		return nil, err
	}

	scored := weightedRRFMerge(fusion.RRFK, []float64{fusion.WeightFTS, fusion.WeightANN}, ftsIDs, annIDs)

	if len(scored) > int(pool) {
		return scored[:pool], nil
	}
	return scored, nil
}

func rrfMerge(rankings ...[]ScoredChunkID) []ScoredChunkID {
	weights := make([]float64, len(rankings))
	for i := range weights {
		weights[i] = 1
	}
	return weightedRRFMerge(rrfK, weights, rankings...)
}

func weightedRRFMerge(k int64, weights []float64, rankings ...[]ScoredChunkID) []ScoredChunkID {
	scores := make(map[int64]float64)

	for i, ranking := range rankings {
		for idx, chunkID := range ranking {
			rank := int64(idx) + 1
			scores[chunkID.ID] += weights[i] / float64(k+rank)
		}
	}

	scored := make([]ScoredChunkID, 0, len(scores))
	for id, score := range scores {
		scored = append(scored, ScoredChunkID{
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

	for i := range scored {
		scored[i].Rank = int64(i + 1)
	}

	return scored
}

func sourceRerank(chunks []RetrievedChunk) []RetrievedChunk {
	weighted := make([]RetrievedChunk, 0, len(chunks))
	for _, c := range chunks {
		c.Score *= c.SourceWeight
		weighted = append(weighted, c)
	}

	sort.Slice(weighted, func(i, j int) bool {
		if weighted[i].Score == weighted[j].Score {
			return weighted[i].ID < weighted[j].ID
		}
		return weighted[i].Score > weighted[j].Score
	})
	for i := range weighted {
		weighted[i].Rank = int64(i + 1)
	}

	return weighted
}
