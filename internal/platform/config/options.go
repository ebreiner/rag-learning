package config

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Option struct {
	Name      string
	Default   string
	FlagLong  string
	FlagShort string
	FlagUsage string
	IsPath    bool
}

type Globals struct {
	XBergURL       Option
	OpenAIEmbedURL Option
	DoclingURL     Option
	DBPath         Option
	LogPath        Option
}

var GlobalOptions = Globals{
	OpenAIEmbedURL: Option{FlagLong: "openai-url", FlagUsage: "--openai-url https://stuff.foo.net/openai-compatible-endpoint", Default: "http://127.0.0.1:11434", IsPath: false},
	XBergURL:       Option{FlagLong: "xberg-url", FlagUsage: "--xberg-url https://stuff.foo.net/kreuzberg", Default: "http://localhost:8000", IsPath: false},
	DoclingURL:     Option{FlagLong: "docling-url", FlagUsage: "--docling-url https://stuff.foo.net/docling", Default: "http://localhost:5001", IsPath: false},
	DBPath:         Option{FlagLong: "db-path", FlagUsage: "--db-path /path/to/db", Default: "./data.db", IsPath: true},
	LogPath:        Option{FlagLong: "log-path", FlagUsage: "--log-path /path/to/foo.log", Default: "./rag-cli.log", IsPath: true},
}

func GlobalsList() []Option {
	return []Option{
		GlobalOptions.DBPath,
		GlobalOptions.OpenAIEmbedURL,
		GlobalOptions.XBergURL,
		GlobalOptions.DoclingURL,
		GlobalOptions.LogPath,
	}
}

func RegisterFlags(flagSet *pflag.FlagSet, options []Option) {
	for _, o := range options {
		if o.FlagShort != "" {
			flagSet.StringP(o.FlagLong, o.FlagShort, "", o.FlagUsage)
		} else {
			flagSet.String(o.FlagLong, "", o.FlagUsage)
		}
	}
}

func ResolveGlobal(cmd *cobra.Command, option Option) (string, error) {
	flag := cmd.Root().Flags().Lookup(option.FlagLong)
	if flag == nil {
		return "", fmt.Errorf("flag '%s' is nil and therefore not registered", option.FlagLong)
	}
	var value string
	if flag.Changed {
		value = flag.Value.String()
		if value == "" {
			return value, fmt.Errorf("empty flag '%s'", option.FlagLong)
		}
	} else {
		value = option.Default
	}

	if option.IsPath {
		abs, err := ResolvePath(value)
		if err != nil {
			return "", err
		}

		value = abs
	}

	return value, nil
}

func ResolvePath(toResolve string) (string, error) {
	abs, err := filepath.Abs(toResolve)
	if err != nil {
		return "", err
	} else {
		return abs, nil
	}
}
