// Command wikiscrape downloads all articles of a MediaWiki and converts them to Markdown with pandoc.
//
// It is a standalone scratch tool for one-time use. It does not import the rag application packages.
// It is restored from the old cmd/scrape command (removed in commit 771d78a).
package main

import (
	"log"
	"net/url"
	"os"
	"time"

	"github.com/spf13/cobra"
)

const passwordEnv = "MW_PASSWORD"

var rootCmd = &cobra.Command{
	Use:   "wikiscrape",
	Short: "Download all articles of a MediaWiki and convert them to Markdown with pandoc",
	Long: `wikiscrape downloads all pages of the main namespace (ns 0) of a MediaWiki
through its API. It writes the wikitext of each page to <title>.mw in the output
directory. Then it converts each .mw file to <title>.md with pandoc.

Use this tool only on a wiki that you have permission to download.
Do not commit the downloaded content.

Login is optional. Set --user to log in. The tool reads the password from the
environment variable ` + passwordEnv + `. If it is empty, the tool asks for the password.
Use a MediaWiki bot password (Special:BotPasswords) for the API login.`,
	Example: `  wikiscrape --mw-address https://wiki.example.org/api.php --output-dir ./data/wiki
  wikiscrape -a https://wiki.example.org/api.php -u ExampleUser@scrape --delay 1s
  wikiscrape --skip-download --output-dir ./data/wiki
  wikiscrape -a https://wiki.example.org/api.php -u ExampleUser@scrape --meta --skip-download --skip-convert`,
	Args: cobra.NoArgs,
	RunE: run,
}

func init() {
	f := rootCmd.Flags()
	f.BoolP("skip-download", "s", false, "do not download; only convert the .mw files in the output directory")
	f.StringP("output-dir", "o", "./data/output", "output directory for the .mw and .md files")
	f.StringP("mw-address", "a", "", "MediaWiki API URL, for example https://wiki.example.org/api.php")
	f.StringP("user", "u", "", "user name for the API login (optional; no login when empty)")
	f.Duration("delay", 500*time.Millisecond, "delay before each API request")
	f.String("user-agent", "wikiscrape/0.1 (one-time export; contact: you@example.org)", "User-Agent header for each API request")
	f.String("to", "gfm", "pandoc output format, for example gfm or markdown")
	f.Bool("meta", false, "also download the page metadata (last edit, size, categories) to "+metaFile+"; no user data")
	f.Bool("skip-convert", false, "do not convert the .mw files with pandoc")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(command *cobra.Command, _ []string) error {
	f := command.Flags()
	skipDownload, _ := f.GetBool("skip-download")
	outputDir, _ := f.GetString("output-dir")
	address, _ := f.GetString("mw-address")
	user, _ := f.GetString("user")
	delay, _ := f.GetDuration("delay")
	userAgent, _ := f.GetString("user-agent")
	to, _ := f.GetString("to")
	meta, _ := f.GetBool("meta")
	skipConvert, _ := f.GetBool("skip-convert")

	outputDir, err := OutputDirHelper(outputDir)
	if err != nil {
		return err
	}

	if !skipDownload || meta {
		if address == "" {
			log.Fatal("error: missing mediawiki api url, set --mw-address")
		}
		mwAddress, err := url.Parse(address)
		if err != nil {
			log.Fatalf("error parsing provided mw address %s: %s", address, err.Error())
		}
		c := newClient(userAgent, delay)
		if user != "" {
			c.login(*mwAddress, user, readPassword())
		}
		if meta {
			writePageMeta(c.getPageMeta(*mwAddress), outputDir)
		}
		if !skipDownload {
			pages := c.getArticleList(*mwAddress)
			c.getArticles(pages, *mwAddress, outputDir)
		}
	}
	if !skipConvert {
		articlesToMarkdown(outputDir, to)
	}
	return nil
}
