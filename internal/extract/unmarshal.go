package extract

import (
	"encoding/json"
)

func handleRequestResult(respResults []byte) ([]Document, error) {
	// keep response struct scoped to this function
	// dump response into a file and feed it to some llm to update or add new fields
	type requestNodeContent struct {
		NodeType          string `json:"node_type,omitempty"`
		GroupHeadingLevel int32  `json:"heading_level,omitempty"`
		GroupHeadingText  string `json:"heading_text,omitempty"`
		Level             int32  `json:"level,omitempty"`
		Text              string `json:"text,omitempty"`
		Ordered           bool   `json:"ordered,omitempty"`
	}

	type requestDocNode struct {
		ID       string             `json:"id,omitempty"`
		Content  requestNodeContent `json:"content,omitempty"`
		Children []int32            `json:"children,omitempty"`
		Parent   int32              `json:"parent,omitempty"`
		Page     int32              `json:"page,omitempty"`
	}

	type requestDocument struct {
		NodesRequest []requestDocNode `json:"nodes,omitempty"`
		SourceFormat string           `json:"source_format,omitempty"`
	}

	type requestMetadata struct {
		Authors              []string `json:"authors,omitempty"`
		CreatedAt            string   `json:"created_at,omitempty"`
		CreatedBy            string   `json:"created_by,omitempty"`
		FormatType           string   `json:"format_type,omitempty"`
		PDFVersion           string   `json:"pdf_version,omitempty"`
		Producer             string   `json:"producer,omitempty"`
		IsEncrypted          bool     `json:"is_encrypted,omitempty"`
		Width                int32    `json:"width,omitempty"`
		Height               int32    `json:"height,omitempty"`
		PageCount            int32    `json:"page_count,omitempty"`
		ExtractionDurationMs int32    `json:"extraction_duration_ms,omitempty"`
		OutputFormat         string   `json:"output_format,omitempty"`
		QualityScore         float64  `json:"quality_score,omitempty"`
		EmbeddingsGenerated  bool     `json:"embeddings_generated,omitempty"`
		ChunkCount           int32    `json:"chunk_count,omitempty"`
		Title                string   `json:"title,omitempty"`
		Subject              string   `json:"subject,omitempty"`
		ToMail               []string `json:"mail_to,omitempty"`
		FromMail             string   `json:"mail_from,omitempty"`
		CCMail               []string `json:"mail_cc,omitempty"`
	}

	type resultItem struct {
		MimeType     string            `json:"mime_type,omitempty"`
		Metadata     requestMetadata   `json:"metadata,omitempty"`
		Tables       []json.RawMessage `json:"tables,omitempty"`
		Document     requestDocument   `json:"document,omitempty"`
		QualityScore float64           `json:"quality_score,omitempty"`
	}

	var unmarshaled []resultItem
	if err := json.Unmarshal(respResults, &unmarshaled); err != nil {
		return nil, err
	}

	var documents []Document
	for _, result := range unmarshaled {
		document := Document{
			MimeType:     result.MimeType,
			Title:        result.Metadata.Title,
			SourceFormat: result.Document.SourceFormat,
		}

		metaData := MetaData{
			QualityScore: result.QualityScore,
		}
		if result.Document.SourceFormat == "email" {
			metaData.Mail = MailData{
				Subject:  result.Metadata.Subject,
				MailFrom: result.Metadata.FromMail,
				MailTo:   result.Metadata.ToMail,
				MailCC:   result.Metadata.CCMail,
			}
			document.Title = result.Metadata.Subject
		}

		nodes := make([]Node, 0)
		for _, n := range result.Document.NodesRequest {
			node := Node{
				ID:                n.ID,
				NodeType:          n.Content.NodeType,
				Parent:            n.Parent,
				Children:          n.Children,
				GroupHeadingLevel: n.Content.GroupHeadingLevel,
				GroupHeadingText:  n.Content.GroupHeadingText,
				Level:             n.Content.Level,
				Page:              n.Page,
				Text:              n.Content.Text,
			}
			nodes = append(nodes, node)
		}
		document.Nodes = nodes
		documents = append(documents, document)
	}

	return documents, nil
}
