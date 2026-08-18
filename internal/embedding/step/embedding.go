package step

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func Embed(sink EmbeddingsSink, source ChunkSource, client EmbedClient, ctx context.Context, logger *slog.Logger) error {
	var limit int64 = 10

	for {
		tracer := otel.Tracer("rac-cli-sdk")
		stepCtx, stepSpan := tracer.Start(ctx, "embed-step")
		nextCtx, nextSpan := tracer.Start(stepCtx, "next_chunk")
		rawChunks, err := source.NextChunks(limit, nextCtx)

		if err != nil {
			stepSpan.End()
			nextSpan.End()
			return err
		}
		if len(rawChunks) == 0 {
			stepSpan.End()
			nextSpan.End()
			return err
		}
		stepSpan.SetAttributes(
			attribute.Int64("doc.id", rawChunks[0].DocID),
			attribute.Int("doc.chunks.to_embed", len(rawChunks)),
		)

		nextSpan.End()

		var chunks []ChunkToEmbed
		for _, chunk := range rawChunks {
			if len(chunk.Text) <= 1 {
				continue
			} else {
				chunks = append(chunks, chunk)
			}
		}

		embedCtx, embedSpan := tracer.Start(stepCtx, "embed_with_fallback")
		embeddingsToSave, err := embedWithFallback(client, chunks, embedCtx, logger)
		if err != nil {
			embedSpan.End()
			stepSpan.End()
			return err
		}
		embedSpan.End()

		sinkCtx, sinkSpan := tracer.Start(stepCtx, "save_embeddings")
		if len(embeddingsToSave.Embeddings) > 0 {
			if err := sink.SaveEmbeddings(embeddingsToSave, sinkCtx); err != nil {
				sinkSpan.End()
				stepSpan.End()
				return err
			}
		}

		if len(rawChunks) < int(limit) {
			sinkSpan.End()
			stepSpan.End()
			break
		}
		sinkSpan.End()
		stepSpan.End()
	}

	return nil
}

// unexpected behaviour with batching and embedding models made this necessary.
// example: if a batch of an embedding response contains to many nearly identical texts,
// this can break the embed model, then they start return NaN and other weird stuff. Most
// experienced problems with different models were fixed by the following stuff:
func embedWithFallback(client EmbedClient, chunks []ChunkToEmbed, ctx context.Context, logger *slog.Logger) (EmbeddingsToSave, error) {
	if len(chunks) == 0 {
		return EmbeddingsToSave{}, nil
	}
	result, err := client.EmbedChunks(chunks, ctx)
	if err == nil {
		return result, nil
	}

	if len(chunks) == 1 {
		// a single chunk failing on its own is a real -> individual problem
		// log and skip it rather than blocking the complete pipe
		logger.WarnContext(ctx, "run-embedding", "warn", fmt.Sprintf("skipping chunk %d, failed to embed even alone: %v \n chunk text: %s", chunks[0].ChunkID, err, chunks[0].Text))
		return EmbeddingsToSave{}, nil
	}

	mid := len(chunks) / 2
	first, err := embedWithFallback(client, chunks[:mid], ctx, logger)
	if err != nil {
		return EmbeddingsToSave{}, err
	}
	second, err := embedWithFallback(client, chunks[mid:], ctx, logger)
	if err != nil {
		return EmbeddingsToSave{}, err
	}

	merged := EmbeddingsToSave{Embeddings: append(first.Embeddings, second.Embeddings...)}
	if len(first.Embeddings) > 0 {
		merged.Model, merged.Dim = first.Model, first.Dim
	} else if len(second.Embeddings) > 0 {
		merged.Model, merged.Dim = second.Model, second.Dim
	}
	return merged, nil
}
