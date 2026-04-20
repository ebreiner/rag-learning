package step

import (
	"errors"
	"fmt"
	"io"
)

func RunExtract(docSource DocSource, docSink DocSink, extractor Extractor) error {
	var outerErr error
	counter := 0
	for {
		counter++
		sourceDoc, err := docSource.NextSourceDoc()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			outerErr = err
			break
		}

		fmt.Printf("processing doc # %d: %s\n", counter, sourceDoc.SourcePath)

		extractedDoc, err := extractor.ExtractSourceDoc(sourceDoc)
		if err != nil {
			outerErr = err
			break
		}

		err = docSink.SaveExtractedDoc(extractedDoc)
		if err != nil {
			outerErr = err
			break
		}
	}

	return outerErr
}
