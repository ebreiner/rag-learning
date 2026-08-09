package step

import "log"

func Embed(sink EmbeddingsSink, source ChunkSource, client EmbedClient) error {
	var limit int64 = 10

	for {
		rawChunks, err := source.NextChunks(limit)
		if err != nil {
			return err
		}
		if len(rawChunks) == 0 {
			return err
		}

		var chunks []ChunkToEmbed
		for _, chunk := range rawChunks {
			if len(chunk.Text) <= 1 {
				continue
			} else {
				chunks = append(chunks, chunk)
			}
		}

		embeddingsToSave, err := embedWithFallback(client, chunks)
		if err != nil {
			return err
		}

		if len(embeddingsToSave.Embeddings) > 0 {
			if err := sink.SaveEmbeddings(embeddingsToSave); err != nil {
				return err
			}
		}

		if len(rawChunks) < int(limit) {
			break
		}
	}

	return nil
}

// unexpected behaviour with batching and embedding models made this necessary.
// example: if a batch of an embedding response contains to many nearly identical texts,
// this can break the embed model, then they start return NaN and other weird stuff. Most
// experienced problems with different models were fixed by the following stuff:
func embedWithFallback(client EmbedClient, chunks []ChunkToEmbed) (EmbeddingsToSave, error) {
	result, err := client.EmbedChunks(chunks)
	if err == nil {
		return result, nil
	}

	if len(chunks) == 1 {
		// a single chunk failing on its own is a real -> individual problem
		// log and skip it rather than blocking the complete pipe
		log.Printf("skipping chunk %d, failed to embed even alone: %v \n chunk text: %s", chunks[0].ChunkID, err, chunks[0].Text)
		return EmbeddingsToSave{}, nil
	}

	mid := len(chunks) / 2
	first, err := embedWithFallback(client, chunks[:mid])
	if err != nil {
		return EmbeddingsToSave{}, err
	}
	second, err := embedWithFallback(client, chunks[mid:])
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
