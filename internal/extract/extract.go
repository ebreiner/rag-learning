package extract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Node struct {
	ID                string
	NodeType          string
	Parent            int32
	Children          []int32
	GroupHeadingLevel int32
	GroupHeadingText  string
	Level             int32
	Text              string
	Page              int32
}

type Document struct {
	Title        string
	Nodes        []Node
	SourceFormat string
	MimeType     string
	Metadata     MetaData
}

type MetaData struct {
	QualityScore float64
	Mail         MailData
}

type MailData struct {
	Subject  string
	MailFrom string
	MailCC   []string
	MailTo   []string
}

type writeError struct {
	OutputPath string
	Err        string
}

func (e writeError) Error() string {
	return fmt.Sprintf("error writing file %s: %s", e.OutputPath, e.Err)
}

// TODO: upload async and stream files with io.Pipe into connections
func Extract(outputDir, inputDir string) error {
	inputDirList, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("error reading input dir: %s", err.Error())
	}

	errChan := make(chan writeError)
	resultChan := make(chan []Document)
	var wgDone sync.WaitGroup
	var wgErr sync.WaitGroup
	errList := make([]writeError, 0)
	go func() {
		for err := range errChan {
			errList = append(errList, err)
		}
	}()

	wgDone.Add(len(inputDirList))
	wgErr.Add(1)
	go handleWrites(outputDir, &wgDone, &wgErr, errChan, resultChan)

	filesInBatch := 0
	batchSize := 10
	body, writer, err := newMultipartForm()
	if err != nil {
		return err
	}
	titleList := make([]string, 0, batchSize)
	for i, inputEntry := range inputDirList {
		fmt.Printf("processing file # %d: %s\n", i, inputEntry.Name())
		titleList = append(titleList, filenameWithoutExt(inputEntry.Name()))
		inputPath := filepath.Join(inputDir, inputEntry.Name())
		err := addFileToMultipart(inputPath, inputEntry.Name(), writer)
		if err != nil {
			return fmt.Errorf("error adding file %s to multipart: %s", inputEntry.Name(), err.Error())
		}

		filesInBatch++

		if filesInBatch == batchSize {
			err = sendBatch(titleList, body, writer, resultChan)
			if err != nil {
				return fmt.Errorf("error sending batch: %s", err.Error())
			}
			// reset list by setting length to zero while keeping capacity
			titleList = titleList[:0]

			body, writer, err = newMultipartForm()
			if err != nil {
				return fmt.Errorf("error constructing new multipart: %s", err.Error())
			}

			filesInBatch = 0
		}

	}

	if filesInBatch > 0 {
		err = sendBatch(titleList, body, writer, resultChan)
		if err != nil {
			return fmt.Errorf("error sending final batch: %s", err.Error())
		}
	}

	close(resultChan)

	wgDone.Wait()
	wgErr.Wait()
	close(errChan)

	if len(errList) > 0 {
		var errString string
		for count, err := range errList {
			errString = errString + fmt.Sprintf("# %d %s: %s\n", count, err.OutputPath, err.Err)
		}
		return fmt.Errorf(errString)
	}
	return nil
}

func handleWrites(outputDir string, wgDone, errDone *sync.WaitGroup, errChan chan writeError, resultChan chan []Document) {
	fmt.Println("writer started and ready..")
	for results := range resultChan {
		for _, result := range results {
			name := result.Title
			if result.SourceFormat == "email" {
				name = result.Metadata.Mail.Subject
			}
			outputFileName := name + ".jsonl"
			outputPath := filepath.Join(outputDir, outputFileName)
			content, err := json.Marshal(result)
			if err != nil {
				errChan <- writeError{
					OutputPath: outputPath,
					Err:        fmt.Sprintf("cannot marshal json in writer: %s", err.Error()),
				}
				wgDone.Done()
			}

			if err := os.WriteFile(outputPath, content, 0640); err != nil {
				errChan <- writeError{
					OutputPath: outputPath,
					Err:        fmt.Sprintf("cannot write file %s: %s", outputPath, err.Error()),
				}
				wgDone.Done()
			}

			wgDone.Done()
		}
	}
	errDone.Done()
}

func newMultipartForm() (*bytes.Buffer, *multipart.Writer, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	configPart, err := writer.CreateFormField("config")
	configBuffer := bytes.NewBufferString(extractConfig)
	_, err = io.Copy(configPart, configBuffer)
	if err != nil {
		return body, writer, fmt.Errorf("error copying config buffer: %s", err.Error())
	}

	return body, writer, nil
}

func addFileToMultipart(filepath, multipartFileName string, writer *multipart.Writer) error {
	f, err := os.Open(filepath)
	if err != nil {
		return fmt.Errorf("cannot open file %s: %s", filepath, err.Error())
	}
	defer f.Close()

	part, err := writer.CreateFormFile("files", multipartFileName)
	if err != nil {
		return fmt.Errorf("error adding file %s to writer: %s", multipartFileName, err.Error())
	}

	if _, err := io.Copy(part, f); err != nil {
		return fmt.Errorf("error copying input file buffer to writer: %s", err.Error())
	}

	return nil
}

func sendBatch(titleList []string, body *bytes.Buffer, writer *multipart.Writer, resultChan chan []Document) error {
	err := writer.Close()
	if err != nil {
		return fmt.Errorf("error closing writer: %s", err.Error())
	}

	req, err := http.NewRequest("POST", "http://127.0.0.1:8000/extract", body)
	if err != nil {
		return fmt.Errorf("error creating request: %s", err.Error())
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("error received for extraction request: %s", err.Error())
	}

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %s", err.Error())
	}
	err = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("cannot close request body: %s", err.Error())
	}

	documents, err := handleRequestResult(result)
	if err != nil {
		return fmt.Errorf("error processing read request body: %s", err.Error())
	}

	for i := range documents {
		if i < len(titleList) {
			documents[i].Title = titleList[i]
		}
	}
	resultChan <- documents

	return nil
}

func filenameWithoutExt(filename string) string {
	split := strings.Split(filename, ".")
	split = split[:len(split)-1]
	var filenameNoExt string
	for _, el := range split {
		filenameNoExt = filenameNoExt + el
	}
	return filenameNoExt
}
