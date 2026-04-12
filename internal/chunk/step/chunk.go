package step

import (
	"errors"
	"fmt"
	"io"
)

func Chunk(source ExtractionSource, sink ResultSink) error {
	var counter int
	var outerErr error
	for {
		counter++
		extract, err := source.NextExtraction()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			fmt.Println("extraction source broke")
			outerErr = err
			break
		}

		err = processDoc(extract, sink)
		if err != nil {
			outerErr = err
			break
		}
	}

	return outerErr
}

func processDoc(extraction ExtractionToChunk, sink ResultSink) error {
	maxSize := 1000
	var chunks []ChunkToSave
	var chunk ChunkToSave

	// local mini helper to ease up emitting of chunks, tightly coupled to the state this scope!
	emitchunk := func() {
		chunk.Position = int64(len(chunks))
		chunks = append(chunks, chunk)
		chunk = ChunkToSave{}
	}

	for _, node := range extraction.Nodes {

		// code group heading image list list_item paragraph table
		switch node.NodeType {
		case "heading":
			if chunk.Text != "" {
				emitchunk()
			}
			chunk.Text = node.Text

		case "paragraph", "list", "list_item", "code":
			if len(chunk.Text)+len(node.Text) >= maxSize {
				emitchunk()
			}
			chunk.Text += node.Text + "\n"

		default:
			continue
		}

	}
	if chunk.Text != "" {
		emitchunk()
	}

	chunksToSave := make([]ChunkToSave, 0, len(chunks))
	for idx, chunk := range chunks {
		toSave := ChunkToSave{Position: int64(idx), Text: chunk.Text}
		chunksToSave = append(chunksToSave, toSave)
	}
	result := ChunkResult{
		ChunksToSave:           chunksToSave,
		ParentRepresentationID: extraction.ParentRepresentationID,
	}
	fmt.Println("still going")

	err := sink.SaveChunks(result)
	if err != nil {
		return err
	}

	return nil
}
