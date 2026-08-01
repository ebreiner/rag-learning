package extractor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"rag/internal/extract/step"
)

type KreuzbergExtractor struct {
	BaseURL string
	Client  *http.Client
}

func NewKreuzbergExtractor(xbergURL string, client *http.Client) (KreuzbergExtractor, error) {
	extractor := KreuzbergExtractor{Client: client}
	_, err := url.Parse(xbergURL)
	if err != nil {
		return extractor, err
	}
	extractor.BaseURL = xbergURL
	return extractor, nil
}

func (e *KreuzbergExtractor) ExtractSourceDoc(sourceDoc step.SourceDoc) (step.ExtractedDoc, error) {
	extDoc := step.ExtractedDoc{}

	body, err := e.sendDocToKreuzberg(sourceDoc, e.BaseURL)
	if err != nil {
		return extDoc, err
	}

	rawDocs, err := unmarshalExtractionResponse(body)
	if err != nil {
		return extDoc, err
	}
	var extRaw rawDoc
	if len(rawDocs) != 1 {
		return extDoc, fmt.Errorf("received not exactly one extracted doc for one source doc")
	} else {
		extRaw = rawDocs[0]
	}

	normalized, err := normalizeDoc(extRaw, sourceDoc)
	if err != nil {
		return extDoc, err
	}

	domainDoc, err := normalizedToDomain(normalized)
	if err != nil {
		return extDoc, err
	} else {
		extDoc = domainDoc
	}

	return extDoc, nil
}

func (e *KreuzbergExtractor) sendDocToKreuzberg(sourceDoc step.SourceDoc, baseURL string) ([]byte, error) {
	var result []byte
	contentType, form, err := encodeIntoForm(sourceDoc.SourcePath)
	if err != nil {
		return result, err
	}

	extractURL, err := url.Parse(baseURL)
	if err != nil {
		return result, err
	}
	extractURL = extractURL.JoinPath("/extract")
	req, err := http.NewRequest("POST", extractURL.String(), form)
	if err != nil {
		return result, err
	}

	req.Header.Set("Content-Type", contentType)

	resp, err := e.Client.Do(req)
	if err != nil {
		return result, err
	}
	if resp.StatusCode != 200 {
		return result, fmt.Errorf("extraction failed: non 200 response code for extraction request")
	}

	result, err = io.ReadAll(resp.Body)
	if err != nil {
		return result, err
	}

	err = resp.Body.Close()
	if err != nil {
		return result, err
	}

	return result, nil
}

func encodeIntoForm(inputPath string) (string, *bytes.Buffer, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	configPart, err := writer.CreateFormField("config")
	if err != nil {
		return "", body, err
	}
	configBuffer := bytes.NewBufferString(extractConfig)
	_, err = io.Copy(configPart, configBuffer)
	if err != nil {
		return "", body, fmt.Errorf(err.Error())
	}

	f, err := os.Open(inputPath)
	if err != nil {
		return "", body, err
	}
	defer f.Close()

	_, name := filepath.Split(inputPath)
	part, err := writer.CreateFormFile("files", name)
	if err != nil {
		return "", body, err
	}

	if _, err := io.Copy(part, f); err != nil {
		return "", body, err
	}

	err = writer.Close()
	if err != nil {
		return "", body, err
	}

	return writer.FormDataContentType(), body, nil
}

func normalizedToDomain(normalized normalizedDocument) (step.ExtractedDoc, error) {
	domainDoc := step.ExtractedDoc{}

	metadata := step.ExtractedMetadata{}
	additional, err := json.Marshal(normalized.Metadata.Additional)
	if err != nil {
		return domainDoc, err
	}
	metadata.Additional = additional
	metadata.MimeType = normalized.Metadata.MimeType
	metadata.QualityScore = normalized.Metadata.QualityScore

	var nodes []step.Node
	for _, nomNode := range normalized.Nodes {
		node := step.Node{
			ID:       nomNode.NodeID,
			NodeType: nomNode.NodeType,
		}

		if nomNode.Text != nil {
			node.Text = nomNode.Text
		}

		if nomNode.Level != nil {
			node.Level = nomNode.Level
		}

		if nomNode.ParentIndex != nil {
			node.ParentIndex = nomNode.ParentIndex
		}

		if nomNode.ChildrenIndexes != nil {
			node.ChildrenIndexes = nomNode.ChildrenIndexes
		}

		if nomNode.Page != nil {
			node.Page = nomNode.Page
		}

		nodes = append(nodes, node)
	}

	domainDoc.Nodes = nodes
	domainDoc.Metadata = metadata
	domainDoc.Source = *normalized.SourceDoc

	return domainDoc, nil
}
