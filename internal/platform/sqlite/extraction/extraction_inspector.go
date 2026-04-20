package extraction

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"rag/internal/platform/sqlite/querries"
	"time"
)

type extractionRow struct {
	ID               int64     `json:"id"`
	CreatedAt        time.Time `json:"created_at"`
	RepresentationID int64     `json:"representation_id"`
	MimeType         string    `json:"mime_type"`
	QualityScore     float64   `json:"quality_score"`
	MetadataJson     string    `json:"metadata_json"`
}

type stats struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type ExtractionInspector struct {
	Limit  int
	Format string
	db     *sql.DB
	ctx    context.Context
}

func NewExtractionInspector(ctx context.Context, db *sql.DB, limit int, format string) ExtractionInspector {
	return ExtractionInspector{
		Limit:  limit,
		Format: format,
		db:     db,
		ctx:    ctx,
	}
}

func (i ExtractionInspector) Stats() (string, error) {
	q := querries.New(i.db)
	count, err := q.CountExtractions(i.ctx)
	if err != nil {
		return "", err
	}
	stats := stats{Type: "extractions", Count: count}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func (i ExtractionInspector) Dump() (string, error) {
	q := querries.New(i.db)
	rows, err := q.HeadExtractions(i.ctx, int64(i.Limit))
	if err != nil {
		return "", err
	}

	exts := make([]extractionRow, 0)
	for _, row := range rows {
		if len(row.QualityScore) != 8 {
			return "", fmt.Errorf("error quality score length is not 8")
		}

		qualityScore := math.Float64frombits(binary.LittleEndian.Uint64(row.QualityScore))

		ext := extractionRow{
			ID:               row.ID,
			CreatedAt:        row.CreatedAt,
			RepresentationID: row.RepresentationID,
			MimeType:         row.MimeType,
			QualityScore:     qualityScore,
		}
		if row.MetadataJson.Valid {
			ext.MetadataJson = row.MetadataJson.String
		}

		exts = append(exts, ext)
	}

	var output string
	switch i.Format {
	case "json":
		s, err := json.Marshal(exts)
		if err != nil {
			return "", err
		}
		output = string(s)
	case "jsonl":
		for _, ext := range exts {
			s, err := json.Marshal(ext)
			if err != nil {
				return "", err
			}
			output = output + string(s)
		}
	default:
		return "", fmt.Errorf("unknown output format '%s'", i.Format)
	}
	return output, nil
}
