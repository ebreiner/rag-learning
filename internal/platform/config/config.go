package config

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var knownStrategies = []string{"fts", "ann", "hybrid"}

func Load(ctx context.Context, path string) (Config, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", path, err)
	}

	raw, err := parse(ctx, absPath)
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", absPath, err)
	}

	cnf, err := resolve(ctx, raw, filepath.Dir(absPath))
	if err != nil {
		return Config{}, fmt.Errorf("config %s: %w", absPath, err)
	}

	if err := validate(ctx, cnf); err != nil {
		return Config{}, fmt.Errorf("config %s: %w", absPath, err)
	}

	return cnf, nil
}

func parse(ctx context.Context, path string) (raw rawConfig, err error) {
	f, err := os.Open(path)
	if err != nil {
		return raw, err
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}()

	dec := yaml.NewDecoder(f)

	dec.KnownFields(true)
	if err := dec.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return raw, fmt.Errorf("file is empty")
		}
		return raw, fmt.Errorf("parsing yaml: %w", err)
	}

	return raw, nil
}

// resolve paths and urls, relative paths resolve against baseDir (dir of config file)
func resolve(ctx context.Context, cnf rawConfig, baseDir string) (Config, error) {
	var errs []error
	fail := func(field string, err error) { errs = append(errs, fmt.Errorf("%s: %w", field, err)) }

	parseURL := func(field, raw string) *url.URL {
		if raw == "" {
			return nil
		}
		u, err := url.Parse(raw)
		if err != nil {
			fail(field, err)
			return nil
		}
		return u
	}

	absPath := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(baseDir, p)
	}

	var c Config

	c.Storage.DBPath = absPath(cnf.Storage.DBPath)

	c.Log.AppLog = absPath(cnf.Log.AppLog)
	c.Log.QueryLog = absPath(cnf.Log.QueryLog)

	c.Docling.URL = parseURL("docling.url", cnf.Docling.URL)
	c.Docling.Timeout = cnf.Docling.Timeout
	c.Docling.DumpDir = absPath(cnf.Docling.DumpDir)
	if len(cnf.Docling.Headers) > 0 {
		header := http.Header{}
		for name, value := range cnf.Docling.Headers {
			header.Add(name, value)
		}
		c.Docling.Headers = header
	}

	c.Embedding.Backend = EmbedBackend(cnf.Embedding.Backend)
	c.Embedding.URL = parseURL("embedding.url", cnf.Embedding.URL)
	c.Embedding.Model = cnf.Embedding.Model
	c.Embedding.Dim = cnf.Embedding.Dim
	c.Embedding.Timeout = cnf.Embedding.Timeout

	for _, s := range cnf.Sources {
		c.Sources = append(c.Sources, Source{
			Name:       s.Name,
			Path:       absPath(s.Path),
			Weight:     s.Weight,
			Attributes: s.Attributes,
		})
	}

	c.Retrieval.PoolSize = cnf.Retrieval.PoolSize
	c.Retrieval.Fusion = Fusion{
		RRFK:      cnf.Retrieval.Fusion.RRFK,
		WeightFTS: cnf.Retrieval.Fusion.WeightFTS,
		WeightANN: cnf.Retrieval.Fusion.WeightANN,
	}

	c.Read.MaxChars = cnf.Read.MaxChars

	c.Server.Listen = cnf.Server.Listen

	c.Eval.Golden = absPath(cnf.Eval.Golden)
	c.Eval.RunsDir = absPath(cnf.Eval.RunsDir)
	c.Eval.K = cnf.Eval.K
	c.Eval.Strategies = cnf.Eval.Strategies

	return c, errors.Join(errs...)
}

func validate(ctx context.Context, cnf Config) error {
	var errs []error
	fail := func(field string, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...)))
	}
	required := func(field, value string) {
		if value == "" {
			fail(field, "required")
		}
	}
	checkURL := func(field string, u *url.URL) {
		if u == nil {
			fail(field, "required")
			return
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			fail(field, "scheme must be http or https, got %q", u.Scheme)
		}
		if u.Host == "" {
			fail(field, "host is missing")
		}
		if u.User != nil {
			fail(field, "user info is not permitted")
		}
	}

	required("storage.db_path", cnf.Storage.DBPath)

	required("log.app_log", cnf.Log.AppLog)
	if cnf.Log.QueryLog != "" && cnf.Log.QueryLog == cnf.Log.AppLog {
		fail("log.query_log", "must not be the same file as log.app_log")
	}

	checkURL("docling.url", cnf.Docling.URL)
	if cnf.Docling.Timeout <= 0 {
		fail("docling.timeout", "must be > 0")
	}
	if cnf.Docling.Headers != nil {
		for name, values := range cnf.Docling.Headers {
			if strings.TrimSpace(name) == "" {
				fail("docling.headers", "empty header name")
			}
			for _, v := range values {
				if v == "" {
					fail("docling.headers."+name, "empty value")
				}
			}
		}
	}

	switch cnf.Embedding.Backend {
	case EmbedBackendOpenAI, EmbedBackendKreuzberg:
	case "":
		fail("embedding.backend", "required")
	default:
		fail("embedding.backend", "must be %q or %q, got %q", EmbedBackendOpenAI, EmbedBackendKreuzberg, cnf.Embedding.Backend)
	}
	checkURL("embedding.url", cnf.Embedding.URL)
	// the vec table name needs model and dim for each backend
	required("embedding.model", cnf.Embedding.Model)
	if cnf.Embedding.Dim <= 0 {
		fail("embedding.dim", "must be > 0")
	}
	if cnf.Embedding.Timeout <= 0 {
		fail("embedding.timeout", "must be > 0")
	}

	if len(cnf.Sources) == 0 {
		fail("sources", "at least one source is required")
	}
	names := map[string]int{}
	for i, s := range cnf.Sources {
		field := fmt.Sprintf("sources[%d]", i)
		required(field+".name", s.Name)
		if prev, ok := names[s.Name]; ok && s.Name != "" {
			fail(field+".name", "%q is already used by sources[%d]", s.Name, prev)
		}
		names[s.Name] = i
		required(field+".path", s.Path)
		if s.Weight <= 0 {
			fail(field+".weight", "must be > 0")
		}
		for j := range i {
			other := cnf.Sources[j].Path
			if s.Path == "" || other == "" {
				continue
			}
			if isSubPath(s.Path, other) || isSubPath(other, s.Path) {
				fail(field+".path", "overlaps with sources[%d].path", j)
			}
		}
	}

	if cnf.Retrieval.PoolSize <= 0 {
		fail("retrieval.pool_size", "must be > 0")
	}
	f := cnf.Retrieval.Fusion
	if f.RRFK <= 0 {
		fail("retrieval.fusion.rrf_k", "must be > 0")
	}
	if f.WeightFTS < 0 {
		fail("retrieval.fusion.weight_fts", "must be >= 0")
	}
	if f.WeightANN < 0 {
		fail("retrieval.fusion.weight_ann", "must be >= 0")
	}
	if f.WeightFTS == 0 && f.WeightANN == 0 {
		fail("retrieval.fusion", "weight_fts and weight_ann must not both be 0")
	}

	if cnf.Read.MaxChars <= 0 {
		fail("read.max_chars", "must be > 0")
	}

	if cnf.Server.Listen == "" {
		fail("server.listen", "required")
	} else if _, port, err := net.SplitHostPort(cnf.Server.Listen); err != nil {
		fail("server.listen", "%v", err)
	} else if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		fail("server.listen", "port must be a number from 1 to 65535, got %q", port)
	}

	required("eval.golden", cnf.Eval.Golden)
	required("eval.runs_dir", cnf.Eval.RunsDir)
	if cnf.Eval.K <= 0 {
		fail("eval.k", "must be > 0")
	}
	if len(cnf.Eval.Strategies) == 0 {
		fail("eval.strategies", "required")
	}
	seen := map[string]bool{}
	for _, s := range cnf.Eval.Strategies {
		if !slices.Contains(knownStrategies, s) {
			fail("eval.strategies", "unknown strategy %q, known: %v", s, knownStrategies)
		}
		if seen[s] {
			fail("eval.strategies", "duplicate strategy %q", s)
		}
		seen[s] = true
	}

	return errors.Join(errs...)
}

func isSubPath(path, parent string) bool {
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
