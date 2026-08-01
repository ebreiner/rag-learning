package step

import (
	"fmt"
)

// TODO: remove all empty string skipping or make it representabl in db, if not removable fully in extractio
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

		var texts []string
		for _, chunk := range chunks {
			if len(chunk.Text) <= 1 {
				continue
			} else {
				texts = append(texts, chunk.Text)
			}
		}
		if len(texts) == 0 {
			continue
		}

		embeddings, err := client.Embed(texts)
		if err != nil {
			return err
		}
		if len(texts) != len(embeddings) {
			return fmt.Errorf("count of embeddings and input texts don't match")
		}

		toSave := make([]EmbeddingToSave, len(embeddings))
		for i := range texts {
			toSave[i] = EmbeddingToSave{
				ChunkID: chunks[i].ChunkID,
				Vector:  embeddings[i],
			}
		}

		err = sink.SaveEmbeddings(toSave)
		if err != nil {
			return err
		}

		if len(rawChunks) < int(limit) {
			break
		}
	}

	return nil
}
