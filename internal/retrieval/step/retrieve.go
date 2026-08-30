package step

import (
	"context"
	"fmt"
	"log/slog"
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
		chunkIDs, err = runHybrid(retrieveCtx, query, k, deps.EmbeddingsClient, deps.Retriever, deps.CollWeigher, deps.Logger)
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

func runHybrid(ctx context.Context, query string, k int64, embed EmbedClient, retriever TopKRetriever, collweigher CollectionWeigher, logger *slog.Logger) ([]ScoredChunkID, error) {
	hybridK := k * 4

	ftsIDs, err := runFTS(ctx, query, hybridK, retriever)
	if err != nil {
		return nil, err
	}

	annIDs, err := runANN(ctx, query, hybridK, embed, retriever)
	if err != nil {
		return nil, err
	}

	scored := rrfMerge(ftsIDs, annIDs)

	collWeights, err := collweigher.WeighChunks(ctx, scored)
	if err != nil {
		return nil, err
	}
	collectionReranked := collectionRerank(ctx, scored, collWeights, logger)

	if len(collectionReranked) > int(k) {
		return collectionReranked[:k], nil
	} else {
		return collectionReranked, nil
	}
}

func rrfMerge(rankings ...[]ScoredChunkID) []ScoredChunkID {
	scores := make(map[int64]float64)

	for _, ranking := range rankings {
		for idx, chunkID := range ranking {
			rank := int64(idx) + 1
			scores[chunkID.ID] += 1.0 / float64(rrfK+rank)
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

func collectionRerank(ctx context.Context, chunks []ScoredChunkID, weights map[int64]float64, logger *slog.Logger) []ScoredChunkID {
	filtered := make([]ScoredChunkID, 0, len(chunks))
	for _, c := range chunks {
		if weight, ok := weights[c.ID]; ok {
			c.Score *= weight
			filtered = append(filtered, c)
		} else {
			logger.WarnContext(ctx, "collection-rerank", "warn", fmt.Sprintf("dropping chunk %d: no collection weight found", c.ID))
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Score == filtered[j].Score {
			return filtered[i].ID < filtered[j].ID
		}
		return filtered[i].Score > filtered[j].Score
	})
	for i := range filtered {
		filtered[i].Rank = int64(i + 1)
	}

	return filtered
}
