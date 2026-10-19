package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// metaFile is the name of the page metadata file in the output directory.
// It does not end in .meta.json, so the ingest does not read it as a sidecar.
const metaFile = "_wikimeta.json"

// PageMeta is the metadata of one page. It contains no user names, e-mail addresses or other editor data.
type PageMeta struct {
	PageID     uint32   `json:"pageid"`
	Title      string   `json:"title"`
	File       string   `json:"file"`      // file name without extension, as written by getArticles
	LastEdit   string   `json:"last_edit"` // timestamp of the latest revision, ISO 8601
	Length     int      `json:"length"`    // page size in bytes
	Categories []string `json:"categories"`
}

// metaPage is the API form of one page. The revisions ask only for the timestamp.
type metaPage struct {
	PageID    uint32 `json:"pageid"`
	Title     string `json:"title"`
	Length    int    `json:"length"`
	Revisions []struct {
		Timestamp string `json:"timestamp"`
	} `json:"revisions"`
	Categories []struct {
		Title string `json:"title"`
	} `json:"categories"`
}

// getPageMeta returns the metadata of all non-redirect pages of the main namespace.
// It uses the allpages generator with 50 pages in each batch and follows the API continuation.
// A page can appear in more than one response (for example when its categories continue), so the parts are merged by page ID.
func (c *client) getPageMeta(baseURL url.URL) []PageMeta {
	params := url.Values{}
	params.Add("action", "query")
	params.Add("generator", "allpages")
	params.Add("gapnamespace", "0")
	params.Add("gapfilterredir", "nonredirects")
	params.Add("gaplimit", "50")
	params.Add("prop", "info|revisions|categories")
	params.Add("rvprop", "timestamp") // only the timestamp: no user, no comment
	params.Add("cllimit", "max")
	params.Add("format", "json")
	params.Add("formatversion", "2")

	byID := map[uint32]*PageMeta{}
	var cont map[string]string
	for batchCounter := 0; ; batchCounter++ {
		q := url.Values{}
		for k, v := range params {
			q[k] = v
		}
		for k, v := range cont {
			q.Set(k, v)
		}

		fmt.Printf("running meta batch # %d\n", batchCounter)
		body := c.do(http.MethodGet, baseURL, q)

		var resp struct {
			Continue map[string]string `json:"continue"`
			Query    struct {
				Pages []metaPage `json:"pages"`
			} `json:"query"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			log.Fatalf("error while json unmarshal of meta batch: %s", err.Error())
		}
		mergePageMeta(byID, resp.Query.Pages)

		if len(resp.Continue) == 0 {
			break
		}
		cont = resp.Continue
	}

	metas := make([]PageMeta, 0, len(byID))
	for _, m := range byID {
		sort.Strings(m.Categories)
		metas = append(metas, *m)
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Title < metas[j].Title })
	fmt.Printf("meta end hit, %d pages\n", len(metas))
	return metas
}

// mergePageMeta adds the parts of one API response to byID.
func mergePageMeta(byID map[uint32]*PageMeta, pages []metaPage) {
	for _, p := range pages {
		m, ok := byID[p.PageID]
		if !ok {
			m = &PageMeta{PageID: p.PageID, Title: p.Title, File: normalizeTitle(p.Title), Categories: []string{}}
			byID[p.PageID] = m
		}
		if p.Length > 0 {
			m.Length = p.Length
		}
		if len(p.Revisions) > 0 && p.Revisions[0].Timestamp != "" {
			m.LastEdit = p.Revisions[0].Timestamp
		}
		for _, cat := range p.Categories {
			name := cat.Title
			if i := strings.Index(name, ":"); i >= 0 {
				name = name[i+1:] // remove the namespace prefix, for example "Kategorie:"
			}
			m.Categories = append(m.Categories, name)
		}
	}
}

// writePageMeta writes the metadata of all pages to _wikimeta.json in outputDir.
func writePageMeta(metas []PageMeta, outputDir string) {
	data, err := json.MarshalIndent(metas, "", "  ")
	if err != nil {
		log.Fatalf("error marshalling page metadata: %s", err.Error())
	}
	outputPath := filepath.Join(outputDir, metaFile)
	if err := os.WriteFile(outputPath, data, 0640); err != nil {
		log.Fatalf("error writing %s: %s", outputPath, err.Error())
	}
	fmt.Printf("wrote metadata of %d pages to %s\n", len(metas), outputPath)
}
