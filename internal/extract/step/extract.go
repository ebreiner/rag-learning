package step

import (
	"errors"
	"fmt"
	"io"
	"log"
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

		if sourceDoc.SHA256 == "" {
			return fmt.Errorf("source doc is missing sha256: %s", sourceDoc.Name)
		}

		isDuplicate, err := docSink.ExistsDoc(sourceDoc.SHA256)
		if err != nil {
			return err
		}
		if isDuplicate {
			log.Printf("duplicate doc skipping # %d: %s\n", counter, sourceDoc.SourcePath)
			continue
		}

		log.Printf("processing doc # %d: %s\n", counter, sourceDoc.SourcePath)

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
