package retrieval

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"

	"rag/internal/platform/sqlite/sqlitetest"
	"rag/internal/retrieval/step"
)

func TestReadCatalog(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	insertChunkWithMetadata(t, db, "a", "manual", `{"enerchart_core": true, "level": 2, "score": 0.5, "lang": "de", "tags": ["x"]}`)
	insertChunkWithMetadata(t, db, "b", "manual", `{"enerchart_core": false, "level": 2, "lang": "de"}`)
	insertChunkInSource(t, db, "no metadata", "manual", 1.0)
	insertChunkWithMetadata(t, db, "c", "changelogs", `{"enerchart_core": true}`)
	if _, err := db.Exec(`INSERT INTO sources (name, weight) VALUES ('empty', 1.0)`); err != nil {
		t.Fatalf("inserting source: %v", err)
	}

	got, err := ReadCatalog(ctx, db)
	if err != nil {
		t.Fatalf("ReadCatalog() error = %v", err)
	}

	want := step.Catalog{Sources: []step.CatalogSource{
		{Name: "changelogs", Documents: 1, Keys: []step.CatalogKey{
			{Key: "enerchart_core", Types: []string{"boolean"}, Documents: 1, DistinctValues: 1, Values: []step.CatalogValue{{Value: true, Documents: 1}}},
		}},
		{Name: "empty", Documents: 0, Keys: []step.CatalogKey{}},
		{Name: "manual", Documents: 3, Keys: []step.CatalogKey{
			{Key: "enerchart_core", Types: []string{"boolean"}, Documents: 2, DistinctValues: 2, Values: []step.CatalogValue{{Value: false, Documents: 1}, {Value: true, Documents: 1}}},
			{Key: "lang", Types: []string{"text"}, Documents: 2, DistinctValues: 1, Values: []step.CatalogValue{{Value: "de", Documents: 2}}},
			{Key: "level", Types: []string{"integer"}, Documents: 2, DistinctValues: 1, Values: []step.CatalogValue{{Value: int64(2), Documents: 2}}},
			{Key: "score", Types: []string{"real"}, Documents: 1, DistinctValues: 1, Values: []step.CatalogValue{{Value: 0.5, Documents: 1}}},
			{Key: "tags", Types: []string{"array"}, Documents: 1, DistinctValues: 1},
		}},
	}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("catalog (-want +got):\n%s", diff)
	}
}
