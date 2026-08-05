package docling

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

type DoclingExtractor struct {
	BaseURL string
	Client  *http.Client
}

func NewDoclingExtractor(doclingURL string, client *http.Client) (DoclingExtractor, error) {
	if _, err := url.Parse(doclingURL); err != nil {
		return DoclingExtractor{}, fmt.Errorf("malformed docling url '%s': %s", doclingURL, err)
	}
	return DoclingExtractor{
		Client:  client,
		BaseURL: doclingURL,
	}, nil
}

func (e *DoclingExtractor) ExtractSourceDoc(sourceDoc step.SourceDoc) (step.ExtractedDoc, error) {
	rawDoc, err := doclingConvert(e.Client, sourceDoc.SourcePath, e.BaseURL)
	if err != nil {
		return step.ExtractedDoc{}, err
	}

	flatNodes, err := buildNodes(rawDoc)
	if err != nil {
		return step.ExtractedDoc{}, err
	}

	extDoc, err := wireGraph(flatNodes, rawDoc)
	if err != nil {
		return step.ExtractedDoc{}, err
	}

	extDoc.MimeType = rawDoc.Origin.MimeType

	extDoc.Source = sourceDoc

	return extDoc, nil
}

func doclingConvert(client *http.Client, inputPath, doclingURL string) (*rawDoclingDocument, error) {
	contentType, form, err := convertMultipartForm(inputPath)
	if err != nil {
		return nil, err
	}
	doclingURL, err = url.JoinPath(doclingURL, "/v1/convert/file")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, doclingURL, form)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("docling conversion failed, response code %d, error response: %s", resp.StatusCode, string(body))
	}

	var desConvertResp rawConvertResponse
	err = json.Unmarshal(body, &desConvertResp)
	if err != nil {
		return nil, err
	}

	if desConvertResp.Document.JSONContent != nil {
		return desConvertResp.Document.JSONContent, nil
	} else {
		return nil, fmt.Errorf("docling response contains no json format of extracted doc")
	}
}

func convertMultipartForm(inputPath string) (string, *bytes.Buffer, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	formatPart, err := writer.CreateFormField("to_formats")
	if err != nil {
		return "", body, err
	}

	formatValue := bytes.NewBufferString("json")
	_, err = io.Copy(formatPart, formatValue)
	if err != nil {
		return "", body, err
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
