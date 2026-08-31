package step

func mergeAIntoB(a, b ChunkToSave) ChunkToSave {
	merged := ChunkToSave{
		Text:       a.Text + "\n\n" + b.Text,
		Breadcrumb: b.Breadcrumb,
		Position:   b.Position,
		Type:       TypeContent,
	}

	ids := make([]ChunkExtractionNodeID, 0, len(a.ExtractionNodeIDs)+len(b.ExtractionNodeIDs))
	for _, id := range a.ExtractionNodeIDs {
		ids = append(ids, ChunkExtractionNodeID{ExtractionNodeID: id.ExtractionNodeID, Position: int64(len(ids))})
	}
	for _, id := range b.ExtractionNodeIDs {
		ids = append(ids, ChunkExtractionNodeID{ExtractionNodeID: id.ExtractionNodeID, Position: int64(len(ids))})
	}
	merged.ExtractionNodeIDs = ids

	return merged
}

func mergeSmallChunks(chunks []ChunkToSave, threshold int) []ChunkToSave {
	result := make([]ChunkToSave, 0, len(chunks))

	i := 0
	for i < len(chunks) {
		c := chunks[i]

		if c.Type == TypeContent && len(c.Text) < threshold && i+1 < len(chunks) && chunks[i+1].Type == TypeContent {
			merged := mergeAIntoB(c, chunks[i+1])

			consumed := 2

			if len(merged.Text) < threshold && i+2 < len(chunks) && chunks[i+2].Type == TypeContent {
				merged = mergeAIntoB(merged, chunks[i+2])
				consumed = 3
			}

			result = append(result, merged)
			i += consumed
			continue
		}

		result = append(result, c)
		i++
	}

	return result
}
