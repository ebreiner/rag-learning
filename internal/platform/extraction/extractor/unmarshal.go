package extractor

import (
	"encoding/json"
	"fmt"
)

type rawDoc struct {
	MimeType string                      `json:"mime_type"`
	Metadata rawExtractionResultMetadata `json:"metadata"`
	// TODO: Tables
	Document struct {
		Nodes        []rawExtractionNode `json:"nodes"`
		SourceFormat *string             `json:"source_format"`
	} `json:"document"`
	QualityScore float64 `json:"quality_score"`
}

type rawExtractionResultMetadata struct {
	Title           *string   `json:"title"`
	Subject         *string   `json:"subject"`
	CreatedAt       *string   `json:"created_at"`
	CreatedBy       *string   `json:"created_by"`
	FormatType      *string   `json:"format_type"`
	Producer        *string   `json:"producer"`
	IsEncrypted     *bool     `json:"is_encrypted"`
	Width           *int64    `json:"width"`
	Height          *int64    `json:"height"`
	PageCount       *int64    `json:"page_count"`
	MailFrom        *string   `json:"from_email"`
	MailTo          *[]string `json:"to_emails"`
	MailCC          *[]string `json:"cc_emails"`
	MailBCC         *[]string `json:"bcc_emails"`
	MailMessageID   *string   `json:"message_id"`
	MailAttachments *[]string `json:"attachments"`
	MailInReplyTo   *string   `json:"in_reply_to"`
	MailReferences  *string   `json:"references"`
	ContentType     *string   `json:"content_type"`
	MimeVersion     *string   `json:"mime_version"`
	UserAgent       *string   `json:"user_agent"`
	QualityScore    *float64  `json:"quality_score"`
}

type rawExtractionNode struct {
	NodeID  *string `json:"id"`
	Content struct {
		NodeType     *string     `json:"node_type"`
		Entries      *[][]string `json:"entries"`
		HeadingLevel *int64      `json:"heading_level"`
		HeadingText  *string     `json:"heading_text"`
		Level        *int64      `json:"level"`
		Text         *string     `json:"text"`
		Ordered      *bool       `json:"ordered"`
	} `json:"content"`
	ChildrenIndexes *[]int64 `json:"children"`
	ParentIndex     *int64   `json:"parent"`
	Page            *int64   `json:"page"`
	Annotations     []struct {
		Start *int64 `json:"start"`
		End   *int64 `json:"end"`
		Kind  struct {
			AnnotationType *string `json:"annotation_type"`
			URL            *string `json:"url"`
		}
	} `json:"annotations"`
}

func unmarshalExtractionResponse(body []byte) ([]rawDoc, error) {
	var docs []rawDoc
	err := json.Unmarshal(body, &docs)
	if err != nil {
		return docs, fmt.Errorf("cannot unmarshal extration response: %s", err.Error())
	} else {
		return docs, nil
	}

}
