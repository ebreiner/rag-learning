package config

import (
	"net/http"
	"net/url"
	"time"
)

type rawConfig struct {
	Storage struct {
		DBPath string `yaml:"db_path"`
	} `yaml:"storage"`
	Log struct {
		AppLog   string `yaml:"app_log"`
		QueryLog string `yaml:"query_log"`
	} `yaml:"log"`
	Docling struct {
		URL     string            `yaml:"url"`
		Timeout time.Duration     `yaml:"timeout"`
		DumpDir string            `yaml:"dump_dir"`
		Headers map[string]string `yaml:"headers"`
	} `yaml:"docling"`
	Embedding struct {
		Backend string        `yaml:"backend"`
		URL     string        `yaml:"url"`
		Model   string        `yaml:"model"`
		Dim     int64         `yaml:"dim"`
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"embedding"`
	Sources []struct {
		Name       string         `yaml:"name"`
		Path       string         `yaml:"path"`
		Weight     float64        `yaml:"weight"`
		Attributes map[string]any `yaml:"attributes"`
	} `yaml:"sources"`
	Retrieval struct {
		PoolSize int64 `yaml:"pool_size"`
		Fusion   struct {
			RRFK      int64   `yaml:"rrf_k"`
			WeightFTS float64 `yaml:"weight_fts"`
			WeightANN float64 `yaml:"weight_ann"`
		} `yaml:"fusion"`
	} `yaml:"retrieval"`
	Server struct {
		Listen string `yaml:"listen"`
	} `yaml:"server"`
	Eval struct {
		Golden     string   `yaml:"golden"`
		RunsDir    string   `yaml:"runs_dir"`
		K          int64    `yaml:"k"`
		Strategies []string `yaml:"strategies"`
	} `yaml:"eval"`
}

type Config struct {
	Storage   Storage
	Log       Log
	Docling   Docling
	Embedding Embedding
	Sources   []Source
	Retrieval Retrieval
	Server    Server
	Eval      Eval
}

type Storage struct {
	DBPath string
}

type Log struct {
	AppLog   string
	QueryLog string
}

type Docling struct {
	URL     *url.URL
	Timeout time.Duration
	Headers http.Header
	DumpDir string
}

type EmbedBackend string

const (
	EmbedBackendOpenAI    EmbedBackend = "openai"
	EmbedBackendKreuzberg EmbedBackend = "kreuzberg"
)

type Embedding struct {
	Backend EmbedBackend
	URL     *url.URL
	Model   string
	Dim     int64
	Timeout time.Duration
}

type Source struct {
	Name       string
	Path       string
	Weight     float64
	Attributes map[string]any // metadata defaults for each document of the source, a sidecar overrides them
}

type Retrieval struct {
	PoolSize int64 // candidates after fusion that go into hydration and the source weight, > 0
	Fusion   Fusion
}

type Fusion struct {
	RRFK      int64
	WeightFTS float64
	WeightANN float64
}

type Server struct {
	Listen string // host:port, checked with net.SplitHostPort
}

type Eval struct {
	Golden     string
	RunsDir    string
	K          int64
	Strategies []string
}
