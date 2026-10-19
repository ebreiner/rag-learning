package step

import (
	"cmp"
	"fmt"
	"slices"
)

const catalogValueCap = 20

type Catalog struct {
	Sources []CatalogSource `json:"sources"`
}

type CatalogSource struct {
	Name      string       `json:"name"`
	Documents int64        `json:"documents"`
	Keys      []CatalogKey `json:"keys"`
}

type CatalogKey struct {
	Key            string         `json:"key"`
	Types          []string       `json:"types"` // boolean, integer, real, text, object, array, null
	Documents      int64          `json:"documents"`
	DistinctValues int            `json:"distinct_values"`
	Values         []CatalogValue `json:"values,omitempty"`
}

type CatalogValue struct {
	Value     any   `json:"value"`
	Documents int64 `json:"documents"`
}

// SourceCount is the document count of one source.
type SourceCount struct {
	Name      string
	Documents int64
}

// CatalogRow is one group of documents with the same source, key, type and value.
type CatalogRow struct {
	Source    string
	Key       string
	Type      string // boolean, integer, real, text, object, array, null
	Value     any    // bool, int64, float64 or string; nil for object, array and null
	Documents int64
}

// BuildCatalog groups the rows for each source and key. A key lists its values only up to catalogValueCap.
func BuildCatalog(sources []SourceCount, rows []CatalogRow) Catalog {
	keysBySource := map[string]map[string]*CatalogKey{}
	for _, row := range rows {
		keys, ok := keysBySource[row.Source]
		if !ok {
			keys = map[string]*CatalogKey{}
			keysBySource[row.Source] = keys
		}
		key, ok := keys[row.Key]
		if !ok {
			key = &CatalogKey{Key: row.Key}
			keys[row.Key] = key
		}
		if !slices.Contains(key.Types, row.Type) {
			key.Types = append(key.Types, row.Type)
		}
		key.Documents += row.Documents
		key.DistinctValues++
		if row.Value != nil {
			key.Values = append(key.Values, CatalogValue{Value: row.Value, Documents: row.Documents})
		}
	}

	catalog := Catalog{Sources: make([]CatalogSource, 0, len(sources))}
	for _, src := range sources {
		cs := CatalogSource{Name: src.Name, Documents: src.Documents, Keys: []CatalogKey{}}
		for _, key := range keysBySource[src.Name] {
			if key.DistinctValues > catalogValueCap {
				key.Values = nil
			}
			slices.Sort(key.Types)
			slices.SortFunc(key.Values, func(a, b CatalogValue) int {
				return cmp.Or(cmp.Compare(b.Documents, a.Documents), cmp.Compare(fmt.Sprint(a.Value), fmt.Sprint(b.Value)))
			})
			cs.Keys = append(cs.Keys, *key)
		}
		slices.SortFunc(cs.Keys, func(a, b CatalogKey) int { return cmp.Compare(a.Key, b.Key) })
		catalog.Sources = append(catalog.Sources, cs)
	}

	return catalog
}

// Source gives the catalog of one source. A source without documents gives an empty entry.
func (c Catalog) Source(name string) CatalogSource {
	for _, s := range c.Sources {
		if s.Name == name {
			return s
		}
	}
	return CatalogSource{Name: name, Keys: []CatalogKey{}}
}
