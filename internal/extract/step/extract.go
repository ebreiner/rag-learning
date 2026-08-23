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

func RunExtract(ctx context.Context, docSource DocSource, docSink DocSink, extractor Extractor, logger *slog.Logger) error {
	for {
		err := extract(ctx, docSource, docSink, extractor, logger)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if errors.Is(err, DuplicateErr) {
				logger.InfoContext(ctx, "extract-doc", "warn", "duplicate doc")
				continue
			}

			return err
		}
	}

	return nil
}

func extract(ctx context.Context, source DocSource, sink DocSink, extractor Extractor, logger *slog.Logger) error {
	tracer := otel.Tracer("rag-cli-sdk")

	stepCtx, stepSpan := tracer.Start(ctx, "extract-step")
	defer stepSpan.End()

	_, sourceSpan := tracer.Start(stepCtx, "next_source_doc")
	defer sourceSpan.End()
	sourceDoc, err := source.NextSourceDoc()
	if err != nil {
		return err
	}

	if sourceDoc.SHA256 == "" {
		return fmt.Errorf("source doc is missing sha256: %s", sourceDoc.Name)
	}

	stepSpan.SetAttributes(
		attribute.String("doc.name", sourceDoc.Name),
		attribute.String("doc.source_path", sourceDoc.SourcePath),
		attribute.String("doc.sha256", sourceDoc.SHA256),
	)

	isDuplicate, err := sink.ExistsDoc(ctx, sourceDoc.SHA256)
	if err != nil {
		return err
	}
	if isDuplicate {
		stepSpan.SetAttributes(attribute.String("doc.is_duplicate", strconv.FormatBool(true)))
		logger.WarnContext(ctx, "run-extract", "warn", fmt.Sprintf("duplicate doc skipping:  %s", sourceDoc.SourcePath))
		return DuplicateErr
	}
	stepSpan.SetAttributes(attribute.String("doc.is_duplicate", strconv.FormatBool(false)))
	sourceSpan.End()

	logger.InfoContext(ctx, fmt.Sprintf("processing doc: %s", sourceDoc.SourcePath))

	extractCtx, extractSpan := tracer.Start(stepCtx, "extract_source_doc")
	defer extractSpan.End()
	extractedDoc, err := extractor.ExtractSourceDoc(extractCtx, sourceDoc)
	if err != nil {
		return err
	}
	extractSpan.End()

	saveCtx, saveSpan := tracer.Start(stepCtx, "save_extracted_doc")
	defer saveSpan.End()
	docID, err := sink.SaveExtractedDoc(saveCtx, extractedDoc)
	if err != nil {
		return err
	}
	stepSpan.SetAttributes(attribute.Int64("doc.id", docID))
	saveSpan.End()

	return nil
}
