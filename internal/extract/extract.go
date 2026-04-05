package extract

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"rag/internal/writer"
	"strings"
	"sync"
)

// TODO: upload async and stream files with io.Pipe into connections
func Extract(outputDir, inputDir string) error {
	inputDirList, err := os.ReadDir(inputDir)
	if err != nil {
		return fmt.Errorf("error reading input dir: %s", err.Error())
	}

	errChan := make(chan writer.WriteError)
	resultChan := make(chan writer.ResultMessage)
	var wgWriter sync.WaitGroup
	wgWriter.Add(1)
	var wgErr sync.WaitGroup
	wgErr.Add(1)

	errList := make([]writer.WriteError, 0)
	go func() {
		for err := range errChan {
			errList = append(errList, err)
		}
		wgErr.Done()
	}()

	go writer.HandleWrites(outputDir, &wgWriter, errChan, resultChan)

	filesInBatch := 0
	batchSize := 10
	body, writer, err := newMultipartForm()
	if err != nil {
		return err
	}
	var wgSender sync.WaitGroup
	doneCount := len(inputDirList) / batchSize
	if len(inputDirList)%batchSize != 0 {
		doneCount = doneCount + 1
	}
	wgSender.Add(doneCount)
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
			go sendBatch(titleList, body, writer, resultChan, errChan, &wgSender)

			// reset list by setting length to zero while keeping capacity
			titleList = make([]string, 0, batchSize)

			body, writer, err = newMultipartForm()
			if err != nil {
				return fmt.Errorf("error constructing new multipart: %s", err.Error())
			}

			filesInBatch = 0
		}

	}

	if filesInBatch > 0 {
		go sendBatch(titleList, body, writer, resultChan, errChan, &wgSender)
		if err != nil {
			return fmt.Errorf("error sending final batch: %s", err.Error())
		}
	}
	wgSender.Wait()
	close(resultChan)
	fmt.Print("writer channel closed, waiting for shutdown..\n")
	wgWriter.Wait()

	wgErr.Wait()
	if len(errList) > 0 {
		var errString string
		for count, err := range errList {
			errString = errString + fmt.Sprintf("# %d %s: %s\n", count, err.OutputPath, err.Err)
		}
		return fmt.Errorf(errString)
	}
	return nil
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

func sendBatch(titleList []string, body *bytes.Buffer, multipartWriter *multipart.Writer, resultChan chan writer.ResultMessage, errChan chan writer.WriteError, wg *sync.WaitGroup) {
	errHelper := func(errString string) {
		errChan <- writer.WriteError{
			Err: errString,
		}
	}
	err := multipartWriter.Close()
	if err != nil {
		errHelper(fmt.Sprintf("error closing writer: %s", err.Error()))
		return
	}

	req, err := http.NewRequest("POST", "http://127.0.0.1:8000/extract", body)
	if err != nil {
		errHelper(fmt.Sprintf("error creating request: %s", err.Error()))
		return
	}

	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		errHelper(fmt.Sprintf("error received for extraction request: %s", err.Error()))
		errHelper(fmt.Sprintf("", err.Error()))
		return
	}

	result, err := io.ReadAll(resp.Body)
	if err != nil {
		errHelper(fmt.Sprintf("error reading response body: %s", err.Error()))
		return
	}
	err = resp.Body.Close()
	if err != nil {
		errHelper(fmt.Sprintf("cannot close request body: %s", err.Error()))
		return
	}

	documents, err := handleRequestResult(result)
	if err != nil {
		errHelper(fmt.Sprintf("error processing read request body: %s", err.Error()))
		return
	}

	for i := range documents {
		if i < len(titleList) {
			documents[i].Title = titleList[i]
		}
	}
	msg := writer.ResultMessage{
		Documents:     documents,
		FileExtension: "jsonl",
	}
	resultChan <- msg
	wg.Done()
}

func filenameWithoutExt(filename string) string {
	split := strings.Split(filename, ".")
	split = split[:len(split)-1]
	filenameNoExt := strings.Join(split, ".")
	return filenameNoExt
}
