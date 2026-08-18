package step

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func RunExtract(docSource DocSource, docSink DocSink, extractor Extractor, logger *slog.Logger, ctx context.Context) error {
	var outerErr error
	counter := 0
	for {
		counter++
		tracer := otel.Tracer("rag-cli-sdk")

		stepCtx, stepSpan := tracer.Start(ctx, "extract-step")

		_, sourceSpan := tracer.Start(stepCtx, "next_source_doc")
		sourceDoc, err := docSource.NextSourceDoc()
		if errors.Is(err, io.EOF) {
			sourceSpan.End()
			stepSpan.End()
			return nil
		}
		if err != nil {
			outerErr = err
			sourceSpan.End()
			stepSpan.End()
			break
		}

		if sourceDoc.SHA256 == "" {
			sourceSpan.End()
			stepSpan.End()
			return fmt.Errorf("source doc is missing sha256: %s", sourceDoc.Name)
		}

		stepSpan.SetAttributes(
			attribute.String("doc.name", sourceDoc.Name),
			attribute.String("doc.source_path", sourceDoc.SourcePath),
			attribute.String("doc.sha256", sourceDoc.SHA256),
		)

		isDuplicate, err := docSink.ExistsDoc(sourceDoc.SHA256, ctx)
		if err != nil {
			sourceSpan.End()
			stepSpan.End()
			return err
		}
		if isDuplicate {
			stepSpan.SetAttributes(attribute.String("doc.is_duplicate", strconv.FormatBool(true)))
			logger.WarnContext(ctx, "run-extract", "warn", fmt.Sprintf("duplicate doc skipping # %d: %s", counter, sourceDoc.SourcePath))
			sourceSpan.End()
			stepSpan.End()
			continue
		}
		stepSpan.SetAttributes(attribute.String("doc.is_duplicate", strconv.FormatBool(false)))
		sourceSpan.End()

		logger.InfoContext(ctx, fmt.Sprintf("processing doc # %d: %s", counter, sourceDoc.SourcePath))

		extractCtx, extractSpan := tracer.Start(stepCtx, "extract_source_doc")
		extractedDoc, err := extractor.ExtractSourceDoc(sourceDoc, extractCtx)
		if err != nil {
			outerErr = err
			extractSpan.End()
			stepSpan.End()
			break
		}
		extractSpan.End()

		saveCtx, saveSpan := tracer.Start(stepCtx, "save_extracted_doc")
		docID, err := docSink.SaveExtractedDoc(extractedDoc, saveCtx)
		if err != nil {
			outerErr = err
			saveSpan.End()
			stepSpan.End()
			break
		}
		stepSpan.SetAttributes(attribute.Int64("doc.id", docID))
		saveSpan.End()
		stepSpan.End()
	}

	return outerErr
}
