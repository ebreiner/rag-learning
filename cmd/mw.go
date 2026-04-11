package cmd

import (
	"bufio"
	"bytes"
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
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var mwCmd = &cobra.Command{
	Use:   "mw",
	Short: "download and convert all articles from given mediawiki url",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		skipDownloadFlag := cmd.Flag("skip-download")
		skipDownload := skipDownloadFlag.Value.String()
		outputDirFlag := cmd.Flag("output-dir")
		outputDir := outputDirFlag.Value.String()
		outputDir, err := outputDirHelper(outputDir)
		if err != nil {
			log.Fatal(err.Error())
		}
		addressFlag := cmd.Flag("mw-address")
		if addressFlag.Value.String() == "empty_placeholder" {
			log.Fatal("error missing mediawiki api url")
		}
		mwAddress, err := url.Parse(addressFlag.Value.String())
		if err != nil {
			log.Fatalf("error parsing provided mw address %s: %s", addressFlag.Value.String(), err.Error())
		}

		var client *http.Client
		var articleList string
		if skipDownload == "false" {
			client = login(mwAddress)
			articleList = getArticleList(mwAddress, client)
		}
		getArticles(articleList, mwAddress, outputDir, client)
		articlesToMarkdown(outputDir)
	},
}

func init() {
	rootCmd.AddCommand(mwCmd)
	mwCmd.Flags().BoolP("skip-download", "s", false, "--skip-download")
	mwCmd.Flags().StringP("output-dir", "o", "./data/output", "--output-dir")
	mwCmd.Flags().StringP("mw-address", "a", "empty_placeholder", "--address")
}

type Page struct {
	ID        uint32 `json:"pageid"`
	Namespace uint32 `json:"ns"`
	Title     string `json:"title"`
	Revisions []struct {
		Content string `json:"content"`
	}
}

func login(baseURL *url.URL) *http.Client {
	usernameReader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter username: ")
	username, _ := usernameReader.ReadString('\n')
	username = strings.TrimSuffix(username, "\n")
	fmt.Print("Enter password: ")
	passwordSlice, readPasswordErr := term.ReadPassword(int(syscall.Stdin))
	if readPasswordErr != nil {
		log.Fatalf("error reading in password from stdin: %s", readPasswordErr.Error())
	}
	password := strings.TrimSuffix(string(passwordSlice), "\n")

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Get csrf token for login
	params := url.Values{}
	params.Add("action", "query")
	params.Add("meta", "tokens")
	params.Add("type", "login")
	params.Add("format", "json")

	tokenURL := baseURL
	tokenURL.RawQuery = params.Encode()
	tokenRes, err := client.Get(tokenURL.String())
	if err != nil {
		log.Fatal(err)
	}
	body, _ := io.ReadAll(tokenRes.Body)
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

	loginURL := baseURL
	loginResp, err := client.Post(loginURL.String(), "application/x-www-form-urlencoded", bytes.NewBufferString(loginData.Encode()))
	if err != nil {
		log.Fatalf("cannot login: %s", err.Error())
	}
	// TODO: funktioniert nicht mit status-code, daher result von rpc call unmarshallen und darauf prüfen
	if loginResp.StatusCode != 200 {
		log.Fatal("status code of login is %d and not 200, probably not right..", loginResp.StatusCode)
	}

	loginRespBody, _ := io.ReadAll(loginResp.Body)
	loginResp.Body.Close()

	fmt.Printf("login response: %s", string(loginRespBody))

	// verify
	verifyParams := url.Values{}
	verifyParams.Add("action", "query")
	verifyParams.Add("meta", "userinfo")
	verifyParams.Add("uiprop", "rights")
	verifyParams.Add("format", "json")

	verifyURL := baseURL
	verifyURL.RawQuery = verifyParams.Encode()
	verifyResp, err := client.Get(verifyURL.String())
	if err != nil {
		log.Fatalf("verification of successfull login failed: %s", err.Error())
	}
	verifyBody, _ := io.ReadAll(verifyResp.Body)
	verifyResp.Body.Close()
	fmt.Printf("verification of login successfull, userinfo: %s", string(verifyBody))

	return client
}

func getArticleList(baseURL *url.URL, client *http.Client) string {
	params := url.Values{}
	params.Add("action", "query")
	params.Add("list", "allpages")
	params.Add("format", "json")
	params.Add("formatversion", "2")
	baseURL.RawQuery = params.Encode()

	continueFrom := ""
	var pages []Page
	batchCounter := 0
	for {
		pagesURL := baseURL

		if continueFrom != "" {
			ogQuery := pagesURL.Query()
			ogQuery.Add("apcontinue", continueFrom)
			pagesURL.RawQuery = ogQuery.Encode()
		}

		fmt.Printf("running batch # %d from %s\n", batchCounter, continueFrom)
		resp, err := client.Get(pagesURL.String())
		if err != nil {
			log.Fatalf("error fetching pages list: %s", err.Error())
		}
		body, _ := io.ReadAll(resp.Body)

		var pageResponse struct {
			Continue struct {
				ApContinue string `json:"apcontinue"`
				Continue   string `json:"continue"`
			} `json:"continue"`
			Query struct {
				AllPages []Page `json:"allpages"`
			} `json:"query"`
		}

		if err := json.Unmarshal(body, &pageResponse); err != nil {
			log.Fatalf("error while json unmarshal: %s", err.Error())
		}

		pages = append(pages, pageResponse.Query.AllPages...)
		batchCounter = batchCounter + 1

		// Crazy media wiki pagination ende: https://www.mediawiki.org/wiki/API:Continue
		if !strings.Contains(pageResponse.Continue.Continue, "||") {
			fmt.Printf("pagination end hit, so continue should not contain '||': '%s' ", pageResponse.Continue.Continue)
			break
		}

		continueFrom = pageResponse.Continue.ApContinue

	}
	pageList := ""
	for _, page := range pages {
		pageList = pageList + fmt.Sprintf("{\"pageid\": %d, \"title\": \"%s\", \"ns\": %d}\n", page.ID, page.Title, page.Namespace)
	}

	return pageList
}

func articlesToMarkdown(outputDir string) {
	testPandoc := exec.Command("pandoc", "--version")
	testPandoc.Stdout = os.Stdout
	testPandoc.Stderr = os.Stderr
	if err := testPandoc.Run(); err != nil {
		fmt.Printf("something went wrong checking for pandoc: %s\n", err.Error())
		log.Fatalf("stderr: \n\n%s\n", err.Error())
	}

	inputDir, err := os.ReadDir(outputDir)
	if err != nil {
		log.Fatalf("error reading directory: %s", err.Error())
	}
	for count, inputFileEntry := range inputDir {
		split := strings.Split(inputFileEntry.Name(), ".")
		if split[len(split)-1] != "mw" {
			continue
		}
		title := inputFileEntry.Name()[:len(inputFileEntry.Name())-3]
		title, _ = normalizeTitle(title)
		inputPath := filepath.Join(outputDir, title+".mw")
		outputPath := filepath.Join(outputDir, title+".md")
		fmt.Printf("processing file %s\n", inputPath)

		cmd := exec.Command("pandoc", "-f", "mediawiki", "-t", "markdown", inputPath, "-o", outputPath)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		fmt.Printf("command: %s\n\n", strings.Join(cmd.Args, " "))
		if err := cmd.Run(); err != nil {
			fmt.Printf("error converting mediacode to markdown: %s\n", err.Error())
		} else {
			log.Printf("finished converting # %d %s\n", count, inputPath)
		}
	}
}

func getArticles(articleList string, baseURL *url.URL, outputDir string, client *http.Client) {
	for rowCount, row := range strings.Split(articleList, "\n") {
		if len(row) < 2 {
			fmt.Print("empty line, skipping\n")
			continue
		}
		var page Page
		fmt.Printf("processing page # %d: '%s'\n", rowCount, row)
		if err := json.Unmarshal([]byte(row), &page); err != nil {
			fmt.Printf("row: %s\n\n", row)
			log.Fatalf("error unmarshaling jsonl row %d: %s", rowCount, err.Error())
		}

		params := url.Values{}
		params.Add("titles", page.Title)
		params.Add("action", "query")
		params.Add("prop", "revisions")
		params.Add("rvprop", "content")
		params.Add("format", "json")
		params.Add("formatversion", "2")

		baseURL.RawQuery = params.Encode()
		fmt.Printf("url: %s\n", baseURL.String())
		resp, err := client.Get(baseURL.String())
		if err != nil {
			log.Fatalf("error downloading article '%s': %s", page.Title, err.Error())
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("error reading body of response: %s", err.Error())
		}
		resp.Body.Close()
		type redirectInfo struct {
			From string `json:"from"`
			To   string `json:"to"`
		}

		var pageResponse struct {
			Query struct {
				Page      []Page `json:"pages"`
				Article   string `json:"export"`
				Redirects []struct {
					From string `json:"from"`
					To   string `json:"to"`
				} `json:"redirects"`
			}
		}

		if err := json.Unmarshal(body, &pageResponse); err != nil {
			fmt.Printf("error unmarshalling page response: %s\n", err.Error())
			fmt.Printf("URI: %s\n", baseURL.String())
			log.Fatalf("response: %s\n", string(body))
		}

		if len(pageResponse.Query.Redirects) > 0 {
			page.Title = pageResponse.Query.Redirects[0].To
		}

		if len(pageResponse.Query.Page) != 1 {
			log.Fatal("got more then one page in response, this cannot be right")
		}
		title, _ := normalizeTitle(page.Title)
		title = title + ".mw"
		outputPath := filepath.Join(outputDir, title)
		fmt.Printf("title dump: %s\n", title)
		if len(pageResponse.Query.Page) == 0 {
			log.Fatalf("missing page in response for : %s", baseURL.String())
		}
		if len(pageResponse.Query.Page[0].Revisions) == 0 {
			log.Fatalf("missing revisions for : %s", baseURL.String())
		}
		content := pageResponse.Query.Page[0].Revisions[0].Content
		content = strings.ReplaceAll(content, "<br>", "  ")
		fmt.Printf("writing converted file to %s\n", outputPath)
		if err := os.WriteFile(outputPath, []byte(content), 0640); err != nil {
			log.Fatalf("error writing content of %s to file: %s", outputPath, err.Error())
		}
	}
}

func normalizeTitle(title string) (string, error) {
	normalized := strings.ReplaceAll(title, "/", "||")

	return normalized, nil
}
