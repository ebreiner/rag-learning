package step

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuildCatalog(t *testing.T) {
	t.Run("groups keys for each source, merges the types, sorts keys and values", func(t *testing.T) {
		sources := []SourceCount{{Name: "changelogs", Documents: 10}, {Name: "manual", Documents: 199}}
		rows := []CatalogRow{
			{Source: "manual", Key: "enerchart_core", Type: "boolean", Value: false, Documents: 1},
			{Source: "manual", Key: "enerchart_core", Type: "boolean", Value: true, Documents: 198},
			{Source: "manual", Key: "classification.personal_data", Type: "boolean", Value: false, Documents: 199},
			{Source: "changelogs", Key: "classification.personal_data", Type: "boolean", Value: false, Documents: 10},
			{Source: "manual", Key: "tags", Type: "array", Value: nil, Documents: 3},
		}

		got := BuildCatalog(sources, rows)

		want := Catalog{Sources: []CatalogSource{
			{Name: "changelogs", Documents: 10, Keys: []CatalogKey{
				{Key: "classification.personal_data", Types: []string{"boolean"}, Documents: 10, DistinctValues: 1, Values: []CatalogValue{{Value: false, Documents: 10}}},
			}},
			{Name: "manual", Documents: 199, Keys: []CatalogKey{
				{Key: "classification.personal_data", Types: []string{"boolean"}, Documents: 199, DistinctValues: 1, Values: []CatalogValue{{Value: false, Documents: 199}}},
				{Key: "enerchart_core", Types: []string{"boolean"}, Documents: 199, DistinctValues: 2, Values: []CatalogValue{{Value: true, Documents: 198}, {Value: false, Documents: 1}}},
				{Key: "tags", Types: []string{"array"}, Documents: 3, DistinctValues: 1},
			}},
		}}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("catalog (-want +got):\n%s", diff)
		}
	})

	t.Run("a key above the value cap lists no values, but its count and types", func(t *testing.T) {
		var rows []CatalogRow
		for i := range catalogValueCap + 1 {
			rows = append(rows, CatalogRow{Source: "manual", Key: "title", Type: "text", Value: fmt.Sprintf("t%d", i), Documents: 1})
		}

		got := BuildCatalog([]SourceCount{{Name: "manual", Documents: 21}}, rows)

		key := got.Sources[0].Keys[0]
		if key.Values != nil || key.DistinctValues != catalogValueCap+1 || key.Documents != 21 {
			t.Errorf("key = %+v, want no values, %d distinct values, 21 documents", key, catalogValueCap+1)
		}
	})

	t.Run("a key at the value cap lists all values", func(t *testing.T) {
		var rows []CatalogRow
		for i := range catalogValueCap {
			rows = append(rows, CatalogRow{Source: "manual", Key: "lang", Type: "text", Value: fmt.Sprintf("l%d", i), Documents: 1})
		}

		got := BuildCatalog([]SourceCount{{Name: "manual", Documents: 20}}, rows)

		if n := len(got.Sources[0].Keys[0].Values); n != catalogValueCap {
			t.Errorf("got %d values, want %d", n, catalogValueCap)
		}
	})

	t.Run("a source without documents or metadata has no keys, not nil", func(t *testing.T) {
		got := BuildCatalog([]SourceCount{{Name: "empty", Documents: 0}}, nil)

		if got.Sources[0].Keys == nil || len(got.Sources[0].Keys) != 0 {
			t.Errorf("keys = %#v, want an empty list", got.Sources[0].Keys)
		}
	})
}

func TestCatalogSource(t *testing.T) {
	catalog := Catalog{Sources: []CatalogSource{{Name: "manual", Documents: 3, Keys: []CatalogKey{{Key: "k"}}}}}

	if got := catalog.Source("manual"); got.Documents != 3 || len(got.Keys) != 1 {
		t.Errorf("Source(manual) = %+v, want the manual entry", got)
	}
	got := catalog.Source("other")
	if got.Name != "other" || got.Documents != 0 || got.Keys == nil || len(got.Keys) != 0 {
		t.Errorf("Source(other) = %#v, want an empty entry with an empty key list", got)
	}
}
