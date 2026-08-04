package httpclient

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"
)

type capturingTransport struct {
	next    http.RoundTripper
	dumpDir string
}

func (t *capturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	timestamp := time.Now().Format("2006-01-02T15-04-05")

	reqDump, _ := httputil.DumpRequestOut(req, true)
	regFileName := fmt.Sprintf("%s-req-dump", timestamp)
	regFilePath := filepath.Join(t.dumpDir, regFileName)
	reqFile, reqFileErr := os.OpenFile(regFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if reqFileErr != nil {
		log.Printf("warning: req-dump failed opening file: %s", reqFileErr)
	}
	defer reqFile.Close()
	_, reqFileErr = reqFile.Write(reqDump)

	if reqFileErr != nil {
		log.Printf("warning: req-dump failed writing file: %s", reqFileErr)
	}

	resp, err := t.next.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	respDump, _ := httputil.DumpResponse(resp, true)

	respFileName := fmt.Sprintf("%s-resp-dump", timestamp)
	respFilePath := filepath.Join(t.dumpDir, respFileName)
	respFile, respFileErr := os.OpenFile(respFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if respFileErr != nil {
		log.Printf("warning: resp-dump failed opening file: %s", respFileErr)
	}
	defer respFile.Close()
	_, respFileErr = respFile.Write(respDump)

	if respFileErr != nil {
		log.Printf("warning: resp-dump failed writing file: %s", respFileErr)
	}

	return resp, nil
}

func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

func NewDump(timeout time.Duration, dumpDir string) (*http.Client, error) {
	if err := os.MkdirAll(dumpDir, 0755); err != nil {
		return &http.Client{}, err
	}
	captureingT := &capturingTransport{next: http.DefaultTransport, dumpDir: dumpDir}
	return &http.Client{Timeout: timeout, Transport: captureingT}, nil
}
