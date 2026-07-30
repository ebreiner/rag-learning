package config

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type Option struct {
	Name      string
	Default   string
	FlagLong  string
	FlagShort string
	FlagUsage string
}

type Globals struct {
	XBergURL Option
	DBPath   Option
	LogPath  Option
}

var GlobalOptions = Globals{
	XBergURL: Option{FlagLong: "xberg-url", FlagUsage: "--xberg-url https://stuff.foo.net/kreuzberg", Default: "http://localhost:8000"},
	DBPath:   Option{FlagLong: "db-path", FlagUsage: "--db-path /path/to/db", Default: "./data.db"},
	LogPath:  Option{FlagLong: "log-path", FlagUsage: "--log-path /path/to/foo.log", Default: "./rag-cli.log"},
}

func GlobalsList() []Option {
	return []Option{
		GlobalOptions.DBPath,
		GlobalOptions.XBergURL,
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
		return value, nil
	} else {
		return option.Default, nil
	}
}
