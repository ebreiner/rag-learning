package sqlite_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"rag/internal/platform/sqlite"
	"rag/internal/platform/sqlite/sqlitetest"
)

func TestSetupTable(t *testing.T) {
	t.Run("creates a table named after the sanitized model and dim", func(t *testing.T) {
		db := sqlitetest.New(t)
		got, err := sqlite.SetupVecTable(context.Background(), db, 1024, "bge-m3")
		if err != nil {
			t.Fatalf("SetupTable() error = %v", err)
		}
		want := "embeddings_bge_m3_1024"
		if got != want {
			t.Errorf("table name = %q, want %q", got, want)
		}
	})

	t.Run("sanitizes colons and hyphens in the model name", func(t *testing.T) {
		db := sqlitetest.New(t)
		got, err := sqlite.SetupVecTable(context.Background(), db, 768, "qwen3-embedding:0.6b")
		if err != nil {
			t.Fatalf("SetupTable() error = %v", err)
		}
		if strings.ContainsAny(got, ":-") {
			t.Errorf("table name %q still contains unsanitized characters", got)
		}
	})

	t.Run("the created table actually enforces the declared dimension", func(t *testing.T) {
		db := sqlitetest.New(t)
		tableName, err := sqlite.SetupVecTable(context.Background(), db, 3, "dimtest")
		if err != nil {
			t.Fatalf("SetupTable() error = %v", err)
		}

		packedRight, err := sqlite.PackVector([]float64{1, 2, 3})
		if err != nil {
			t.Fatalf("PackVector: %v", err)
		}
		if _, err := db.Exec("INSERT INTO "+tableName+"(chunk_id, embedding) VALUES (?, ?)", 1, packedRight); err != nil {
			t.Errorf("inserting a correctly-sized vector should succeed, got: %v", err)
		}

		packedWrong, err := sqlite.PackVector([]float64{1, 2, 3, 4, 5})
		if err != nil {
			t.Fatalf("PackVector: %v", err)
		}
		if _, err := db.Exec("INSERT INTO "+tableName+"(chunk_id, embedding) VALUES (?, ?)", 2, packedWrong); err == nil {
			t.Errorf("inserting a wrong-sized vector should be rejected by vec0, but it succeeded")
		}
	})

	t.Run("calling it twice for the same model+dim is idempotent", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		first, err := sqlite.SetupVecTable(ctx, db, 512, "repeat-test")
		if err != nil {
			t.Fatalf("first SetupTable() error = %v", err)
		}
		second, err := sqlite.SetupVecTable(ctx, db, 512, "repeat-test")
		if err != nil {
			t.Fatalf("second SetupTable() error = %v", err)
		}
		if first != second {
			t.Errorf("expected the same table name both times, got %q then %q", first, second)
		}
	})

	t.Run("missing dim errors", func(t *testing.T) {
		db := sqlitetest.New(t)
		if _, err := sqlite.SetupVecTable(context.Background(), db, 0, "some-model"); err == nil {
			t.Error("expected an error for dim=0")
		}
	})

	t.Run("missing model errors", func(t *testing.T) {
		db := sqlitetest.New(t)
		if _, err := sqlite.SetupVecTable(context.Background(), db, 1024, ""); err == nil {
			t.Error("expected an error for an empty model")
		}
	})
}

func TestPackVector(t *testing.T) {
	t.Run("packs a float64 slice without error", func(t *testing.T) {
		packed, err := sqlite.PackVector([]float64{1.5, -2.5, 0, 3.25})
		if err != nil {
			t.Fatalf("PackVector() error = %v", err)
		}
		// float32 packing = 4 bytes per element.
		if len(packed) != 4*4 {
			t.Errorf("packed length = %d, want %d", len(packed), 16)
		}
	})

	t.Run("empty vector packs to empty bytes without error", func(t *testing.T) {
		packed, err := sqlite.PackVector([]float64{})
		if err != nil {
			t.Fatalf("PackVector() error = %v", err)
		}
		if len(packed) != 0 {
			t.Errorf("expected 0 bytes, got %d", len(packed))
		}
	})
}

// sanity check that PackVector's output round-trips through a real vec0
// column correctly -- i.e. that it's not just "some bytes of the right
// length" but bytes vec0 actually interprets as the right float values.
func TestPackVectorRoundTrip(t *testing.T) {
	db := sqlitetest.New(t)
	tableName, err := sqlite.SetupVecTable(context.Background(), db, 3, "roundtrip")
	if err != nil {
		t.Fatalf("SetupTable() error = %v", err)
	}

	want := []float64{1, 2, 3}
	packed, err := sqlite.PackVector(want)
	if err != nil {
		t.Fatalf("PackVector: %v", err)
	}
	if _, err := db.Exec("INSERT INTO "+tableName+"(chunk_id, embedding) VALUES (?, ?)", 1, packed); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// a MATCH query against the exact same vector should return this row
	// at distance ~0 -- proof PackVector's bytes are being interpreted as
	// the intended floats, not garbage that just happens to be the right length.
	var distance sql.NullFloat64
	row := db.QueryRow(
		"SELECT distance FROM "+tableName+" WHERE embedding MATCH ? ORDER BY distance LIMIT 1",
		packed,
	)
	if err := row.Scan(&distance); err != nil {
		t.Fatalf("MATCH query: %v", err)
	}
	if !distance.Valid || distance.Float64 > 0.0001 {
		t.Errorf("distance to itself = %v, want ~0", distance)
	}
}
