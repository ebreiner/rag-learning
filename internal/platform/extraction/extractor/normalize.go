package extractor

import (
	"fmt"
	"rag/internal/extract/step"
	"time"
)

type normalizedDocument struct {
	SourceDoc *step.SourceDoc
	Nodes     []normalizedNode
	Metadata  normalizedMetadata
}

type normalizedNode struct {
	NodeID   string
	NodeType string

	ParentIndex     *int64
	ChildrenIndexes *[]int64
	Page            *int64

	Level *int64
	Text  *string

	Additional normalizedNodeAdditional
}

type normalizedNodeAdditional struct {
	ListOrdered *bool `json:"list_ordered,omitempty"`
	Entries     *[][]string
	Annotations *[]struct {
		Start *int64 `json:"start,omitempty"`
		End   *int64 `json:"end,omitempty"`
		Kind  *struct {
			AnnotationType string `json:"annotation_type,omitempty"`
			URL            string `json:"url,omitempty"`
		}
	} `json:"annotations,omitempty"`
}

type normalizedMetadata struct {
	MimeType     string
	QualityScore float64
	Additional   normalizedMetadataAdditional
}

type normalizedMetadataAdditional struct {
	Title           *string    `json:"title,omitempty"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	CreatedBy       *string    `json:"created_by,omitempty"`
	CreatedWith     *string    `json:"created_with,omitempty"`
	IsEncrypted     *bool      `json:"is_encrypted,omitempty"`
	Width           *int64     `json:"width,omitempty"`
	Height          *int64     `json:"height,omitempty"`
	PageCount       *int64     `json:"page_count,omitempty"`
	MailFrom        *string    `json:"from_email,omitempty"`
	MailTo          *[]string  `json:"to_emails,omitempty"`
	MailCC          *[]string  `json:"cc_emails,omitempty"`
	MailBCC         *[]string  `json:"bcc_emails,omitempty"`
	MailMessageID   *string    `json:"message_id,omitempty"`
	MailAttachments *[]string  `json:"attachments,omitempty"`
	MailInReplyTo   *string    `json:"in_reply_to,omitempty"`
	MailReferences  *string    `json:"references,omitempty"`
	ContentType     *string    `json:"content_type,omitempty"`
	MimeVersion     *string    `json:"mime_version,omitempty"`
	UserAgent       *string    `json:"user_agent,omitempty"`
	QualityScore    *float64   `json:"quality_score"`
	FormatType      *string    `json:"format_type,omitempty"`
}

func normalizeDoc(raw rawDoc, sourceDoc step.SourceDoc) (normalizedDocument, error) {
	normalized := normalizedDocument{
		SourceDoc: &sourceDoc,
	}

	metadata, err := metadataFromRaw(raw)
	if err != nil {
		return normalized, err
	}
	normalized.Metadata = metadata

	normalizedNodes := make([]normalizedNode, 0, len(raw.Document.Nodes))
	for _, rawNode := range raw.Document.Nodes {
		normal, err := normalizedNodeFromRaw(rawNode)
		if err != nil {
			return normalized, err
		}
		normalizedNodes = append(normalizedNodes, normal)
	}
	normalized.Nodes = normalizedNodes

	return normalized, nil
}

func metadataFromRaw(raw rawDoc) (normalizedMetadata, error) {
	metadata := normalizedMetadata{
		MimeType:     raw.MimeType,
		QualityScore: raw.QualityScore,
	}
	additional := normalizedMetadataAdditional{}

	var title string
	if raw.Metadata.Title != nil {
		title = *raw.Metadata.Title
	}
	if raw.Document.SourceFormat != nil && *raw.Document.SourceFormat == "email" && raw.Metadata.Subject != nil {
		title = *raw.Metadata.Subject
	}
	if title != "" {
		additional.Title = &title
	}

	if raw.Metadata.CreatedAt != nil {
		t, err := parseCreatedAt(*raw.Metadata.CreatedAt)
		if err != nil {
			return metadata, fmt.Errorf("error parsing created_at '%s': %s", *raw.Metadata.CreatedAt, err)
		}
		additional.CreatedAt = t
	}

	if raw.Metadata.Producer != nil {
		additional.CreatedWith = raw.Metadata.Producer
	}

	if raw.Metadata.ContentType != nil {
		additional.ContentType = raw.Metadata.ContentType
	}

	if raw.Metadata.CreatedBy != nil {
		additional.CreatedBy = raw.Metadata.CreatedBy
	}

	if raw.Metadata.FormatType != nil {
		additional.FormatType = raw.Metadata.FormatType
	}

	if raw.Document.SourceFormat != nil {
		additional.FormatType = raw.Document.SourceFormat
	}

	if raw.Metadata.Height != nil {
		additional.Height = raw.Metadata.Height
	}

	if raw.Metadata.Width != nil {
		additional.Width = raw.Metadata.Width
	}

	if raw.Metadata.IsEncrypted != nil {
		additional.IsEncrypted = raw.Metadata.IsEncrypted
	}

	if raw.Metadata.MailAttachments != nil {
		additional.MailAttachments = raw.Metadata.MailAttachments
	}

	if raw.Metadata.MailBCC != nil {
		additional.MailBCC = raw.Metadata.MailBCC
	}

	if raw.Metadata.MailCC != nil {
		additional.MailCC = raw.Metadata.MailCC
	}

	if raw.Metadata.MailFrom != nil {
		additional.MailFrom = raw.Metadata.MailFrom
	}

	if raw.Metadata.MailInReplyTo != nil {
		additional.MailInReplyTo = raw.Metadata.MailInReplyTo
	}

	if raw.Metadata.MailMessageID != nil {
		additional.MailMessageID = raw.Metadata.MailMessageID
	}

	if raw.Metadata.MailReferences != nil {
		additional.MailReferences = raw.Metadata.MailReferences
	}

	if raw.Metadata.MailTo != nil {
		additional.MailTo = raw.Metadata.MailTo
	}

	if raw.Metadata.MimeVersion != nil {
		additional.MimeVersion = raw.Metadata.MimeVersion
	}

	if raw.Metadata.PageCount != nil {
		additional.PageCount = raw.Metadata.PageCount
	}

	if raw.Metadata.UserAgent != nil {
		additional.UserAgent = raw.Metadata.UserAgent
	}

	metadata.Additional = additional

	return metadata, nil
}

func normalizedNodeFromRaw(raw rawExtractionNode) (normalizedNode, error) {
	normalNode := normalizedNode{}

	if raw.NodeID == nil {
		return normalNode, fmt.Errorf("raw does not contain mandatory field NodeID")
	}
	normalNode.NodeID = *raw.NodeID

	if raw.Content.NodeType == nil {
		return normalNode, fmt.Errorf("raw does not contain mandatory field NodeType")
	}
	normalNode.NodeType = *raw.Content.NodeType

	if raw.ChildrenIndexes != nil {
		normalNode.ChildrenIndexes = raw.ChildrenIndexes
	}

	if raw.Page != nil {
		normalNode.Page = raw.Page
	}

	if raw.ParentIndex != nil {
		normalNode.ParentIndex = raw.ParentIndex
	}

	if raw.Content.Text != nil {
		normalNode.Text = raw.Content.Text
	}

	if raw.Content.HeadingText != nil {
		normalNode.Text = raw.Content.HeadingText
	}

	if raw.Content.Level != nil {
		normalNode.Level = raw.Content.Level
	}
	if raw.Content.HeadingLevel != nil {
		normalNode.Level = raw.Content.HeadingLevel
	}

	// TODO: annotations
	additional := normalizedNodeAdditional{}
	if raw.Content.Ordered != nil {
		additional.ListOrdered = raw.Content.Ordered
	}

	if raw.Content.Entries != nil {
		additional.Entries = raw.Content.Entries
	}

	normalNode.Additional = additional

	return normalNode, nil
}

func parseCreatedAt(s string) (*time.Time, error) {
	layouts := []string{
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("unsupported time format: %q", s)
}
