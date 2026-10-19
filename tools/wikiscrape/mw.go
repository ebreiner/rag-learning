package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Page struct {
	ID        uint32 `json:"pageid"`
	Namespace uint32 `json:"ns"`
	Title     string `json:"title"`
	Missing   bool   `json:"missing"`
	Revisions []struct {
		Content string `json:"content"`
		Slots   struct {
			Main struct {
				Content string `json:"content"`
			} `json:"main"`
		} `json:"slots"`
	} `json:"revisions"`
}

// client sends the API requests. It keeps the login cookies, sets the User-Agent
// and waits for the delay before each request.
type client struct {
	http      *http.Client
	userAgent string
	delay     time.Duration
}

func newClient(userAgent string, delay time.Duration) *client {
	jar, _ := cookiejar.New(nil)
	return &client{http: &http.Client{Jar: jar, Timeout: 60 * time.Second}, userAgent: userAgent, delay: delay}
}

// do sends one request and returns the body. It stops the program on a transport error or a non-200 status.
func (c *client) do(method string, u url.URL, params url.Values) []byte {
	time.Sleep(c.delay)
	var req *http.Request
	var err error
	if method == http.MethodPost {
		u.RawQuery = ""
		req, err = http.NewRequest(method, u.String(), strings.NewReader(params.Encode()))
		if err == nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	} else {
		u.RawQuery = params.Encode()
		req, err = http.NewRequest(method, u.String(), nil)
	}
	if err != nil {
		log.Fatalf("error building request: %s", err.Error())
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		log.Fatalf("error in %s %s: %s", method, u.Redacted(), err.Error())
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("error reading body of response: %s", err.Error())
	}
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("status code of %s %s is %d and not 200", method, u.Redacted(), resp.StatusCode)
	}
	return body
}

func (c *client) login(baseURL url.URL, username, password string) {
	// Get csrf token for login
	params := url.Values{}
	params.Add("action", "query")
	params.Add("meta", "tokens")
	params.Add("type", "login")
	params.Add("format", "json")

	body := c.do(http.MethodGet, baseURL, params)
	var tokenResponse struct {
		Query struct {
			Tokens struct {
				LoginToken string `json:"logintoken"`
			} `json:"tokens"`
		} `json:"query"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		log.Fatalf("error in unmarshal json: %s", err.Error())
	}
	loginToken := tokenResponse.Query.Tokens.LoginToken

	// Login
	loginData := url.Values{}
	loginData.Add("action", "login")
	loginData.Add("lgname", username)
	loginData.Add("lgpassword", password)
	loginData.Add("lgtoken", loginToken)
	loginData.Add("format", "json")

	loginBody := c.do(http.MethodPost, baseURL, loginData)
	var loginResponse struct {
		Login struct {
			Result string `json:"result"`
			Reason string `json:"reason"`
		} `json:"login"`
	}
	if err := json.Unmarshal(loginBody, &loginResponse); err != nil {
		log.Fatalf("error in unmarshal login response: %s", err.Error())
	}
	if loginResponse.Login.Result != "Success" {
		log.Fatalf("login failed: result %q, reason %q", loginResponse.Login.Result, loginResponse.Login.Reason)
	}

	// verify
	verifyParams := url.Values{}
	verifyParams.Add("action", "query")
	verifyParams.Add("meta", "userinfo")
	verifyParams.Add("format", "json")
	verifyBody := c.do(http.MethodGet, baseURL, verifyParams)
	fmt.Printf("login successful, userinfo: %s\n", string(verifyBody))
}

// getArticleList returns all non-redirect pages of the main namespace. It follows the API continuation.
// See https://www.mediawiki.org/wiki/API:Continue
func (c *client) getArticleList(baseURL url.URL) []Page {
	params := url.Values{}
	params.Add("action", "query")
	params.Add("list", "allpages")
	params.Add("apnamespace", "0")
	params.Add("apfilterredir", "nonredirects")
	params.Add("aplimit", "max")
	params.Add("format", "json")
	params.Add("formatversion", "2")

	var pages []Page
	var cont map[string]string
	for batchCounter := 0; ; batchCounter++ {
		q := url.Values{}
		for k, v := range params {
			q[k] = v
		}
		for k, v := range cont {
			q.Set(k, v)
		}

		fmt.Printf("running batch # %d from %q\n", batchCounter, cont["apcontinue"])
		body := c.do(http.MethodGet, baseURL, q)

		var pageResponse struct {
			Continue map[string]string `json:"continue"`
			Query    struct {
				AllPages []Page `json:"allpages"`
			} `json:"query"`
		}
		if err := json.Unmarshal(body, &pageResponse); err != nil {
			log.Fatalf("error while json unmarshal: %s", err.Error())
		}

		pages = append(pages, pageResponse.Query.AllPages...)

		if len(pageResponse.Continue) == 0 {
			fmt.Printf("pagination end hit, %d pages\n", len(pages))
			break
		}
		cont = pageResponse.Continue
	}
	return pages
}

// getArticles downloads the wikitext of each page and writes it to <title>.mw in outputDir.
func (c *client) getArticles(pages []Page, baseURL url.URL, outputDir string) {
	for count, page := range pages {
		fmt.Printf("processing page # %d: %q\n", count, page.Title)

		params := url.Values{}
		params.Add("titles", page.Title)
		params.Add("action", "query")
		params.Add("prop", "revisions")
		params.Add("rvprop", "content")
		params.Add("rvslots", "main")
		params.Add("format", "json")
		params.Add("formatversion", "2")

		body := c.do(http.MethodGet, baseURL, params)
		var pageResponse struct {
			Query struct {
				Page []Page `json:"pages"`
			} `json:"query"`
		}
		if err := json.Unmarshal(body, &pageResponse); err != nil {
			fmt.Printf("error unmarshalling page response: %s\n", err.Error())
			log.Fatalf("response: %s\n", string(body))
		}

		if len(pageResponse.Query.Page) != 1 {
			log.Fatalf("got %d pages in response for %q, expected 1", len(pageResponse.Query.Page), page.Title)
		}
		p := pageResponse.Query.Page[0]
		if p.Missing || len(p.Revisions) == 0 {
			fmt.Printf("missing page or revisions for %q, skipping\n", page.Title)
			continue
		}
		content := p.Revisions[0].Slots.Main.Content
		if content == "" {
			content = p.Revisions[0].Content
		}
		content = strings.ReplaceAll(content, "<br>", "  ")

		outputPath := filepath.Join(outputDir, normalizeTitle(page.Title)+".mw")
		fmt.Printf("writing file to %s\n", outputPath)
		if err := os.WriteFile(outputPath, []byte(content), 0640); err != nil {
			log.Fatalf("error writing content of %s to file: %s", outputPath, err.Error())
		}
	}
}

// articlesToMarkdown converts each .mw file in outputDir to a .md file with pandoc.
func articlesToMarkdown(outputDir, to string) {
	testPandoc := exec.Command("pandoc", "--version")
	if err := testPandoc.Run(); err != nil {
		log.Fatalf("pandoc not found or not usable: %s", err.Error())
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		log.Fatalf("error reading directory: %s", err.Error())
	}
	count := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".mw" {
			continue
		}
		inputPath := filepath.Join(outputDir, name)
		outputPath := filepath.Join(outputDir, strings.TrimSuffix(name, ".mw")+".md")

		cmd := exec.Command("pandoc", "-f", "mediawiki", "-t", to, inputPath, "-o", outputPath)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			fmt.Printf("error converting %s to markdown: %s\n", inputPath, err.Error())
			continue
		}
		count++
		log.Printf("finished converting # %d %s\n", count, inputPath)
	}
}
