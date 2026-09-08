package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func Embed(ctx context.Context, sink EmbeddingsSink, source ChunkSource, client EmbedClient, logger *slog.Logger) error {
	for {
		if err := embed(ctx, sink, source, client, logger); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return err
		}
	}

	return nil
}

func embed(ctx context.Context, sink EmbeddingsSink, source ChunkSource, client EmbedClient, logger *slog.Logger) error {
	var limit int64 = 10
	tracer := otel.Tracer("rac-cli-sdk")
	stepCtx, stepSpan := tracer.Start(ctx, "embed-step")
	defer stepSpan.End()

	nextCtx, nextSpan := tracer.Start(stepCtx, "next_chunk")
	defer nextSpan.End()
	rawChunks, err := source.NextChunks(nextCtx, limit)

	if err != nil {
		return err
	}
	if len(rawChunks) == 0 {
		return io.EOF
	}
	stepSpan.SetAttributes(
		attribute.Int64("doc.id", rawChunks[0].DocID),
		attribute.Int("doc.chunks.to_embed", len(rawChunks)),
	)

	nextSpan.End()

	embedCtx, embedSpan := tracer.Start(stepCtx, "embed_with_fallback")
	defer embedSpan.End()
	var chunks []ChunkToEmbed
	for _, chunk := range rawChunks {
		if len(chunk.Text) <= 1 {
			continue
		} else {
			chunks = append(chunks, chunk)
		}
	}

	embeddingsToSave, err := embedWithFallback(embedCtx, client, chunks, logger)
	if err != nil {
		return err
	}
	embedSpan.End()

	sinkCtx, sinkSpan := tracer.Start(stepCtx, "save_embeddings")
	defer sinkSpan.End()
	if len(embeddingsToSave.Embeddings) > 0 {
		if err := sink.SaveEmbeddings(sinkCtx, embeddingsToSave); err != nil {
			return err
		}
	}

	if len(rawChunks) < int(limit) {
		return io.EOF
	}
	sinkSpan.End()

	return nil
}

// unexpected behaviour with batching and embedding models made this necessary.
// example: if a batch of an embedding response contains to many nearly identical texts,
// this can break the embed model, then they start return NaN and other weird stuff. Most
// experienced problems with different models were fixed by the following stuff:
func embedWithFallback(ctx context.Context, client EmbedClient, chunks []ChunkToEmbed, logger *slog.Logger) (EmbeddingsToSave, error) {
	if len(chunks) == 0 {
		return EmbeddingsToSave{}, nil
	}
	result, err := client.EmbedChunks(ctx, chunks)
	if err == nil {
		return result, nil
	} else if errors.Is(err, ErrProviderUnreachable) {
		return EmbeddingsToSave{}, err
	}

	if len(chunks) == 1 {
		// a single chunk failing on its own is a real -> individual problem
		// log and skip it rather than blocking the complete pipe
		logger.WarnContext(ctx, "run-embedding", "warn", fmt.Sprintf("skipping chunk %d, failed to embed even alone: %v \n chunk text: %s", chunks[0].ChunkID, err, chunks[0].Text))
		return EmbeddingsToSave{}, nil
	}

	mid := len(chunks) / 2
	first, err := embedWithFallback(ctx, client, chunks[:mid], logger)
	if err != nil {
		return EmbeddingsToSave{}, err
	}
	second, err := embedWithFallback(ctx, client, chunks[mid:], logger)
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
