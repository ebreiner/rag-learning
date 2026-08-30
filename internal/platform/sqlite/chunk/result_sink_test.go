package chunk

import (
	"context"
	"testing"

	"rag/internal/chunk/step"
	"rag/internal/platform/sqlite/sqlitetest"
)

func TestSaveChunks(t *testing.T) {
	t.Run("saves chunk rows with the correct type, text, breadcrumb, and position", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		docID := sqlitetest.InsertDocument(t, db)

		sink, err := NewResultSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewResultSink: %v", err)
		}

		result := step.ChunkResult{
			DocumentID: docID,
			ChunksToSave: []step.ChunkToSave{
				{Text: "content chunk", Breadcrumb: "Ch1", Position: 0, Type: step.TypeContent},
				{Text: "table chunk", Breadcrumb: "Ch1", Position: 1, Type: step.TypeTable},
			},
		}
		if err := sink.SaveChunks(ctx, result); err != nil {
			t.Fatalf("SaveChunks() error = %v", err)
		}

		rows, err := db.Query(`SELECT text, breadcrumb, position, type FROM chunks ORDER BY position`)
		if err != nil {
			t.Fatalf("querying chunks: %v", err)
		}
		defer func() { _ = rows.Close() }()

		type gotChunk struct {
			text, breadcrumb, kind string
			position               int64
		}
		var got []gotChunk
		for rows.Next() {
			var c gotChunk
			if err := rows.Scan(&c.text, &c.breadcrumb, &c.position, &c.kind); err != nil {
				t.Fatalf("scanning chunk row: %v", err)
			}
			got = append(got, c)
		}

		want := []gotChunk{
			{text: "content chunk", breadcrumb: "Ch1", position: 0, kind: "content"},
			{text: "table chunk", breadcrumb: "Ch1", position: 1, kind: "table"},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d chunk rows, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("chunk[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("saves one chunk_nodes row per extraction node id, in order, referencing the right chunk", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		docID := sqlitetest.InsertDocument(t, db)
		node1 := sqlitetest.InsertExtractionNode(t, db)
		node2 := sqlitetest.InsertExtractionNode(t, db)

		sink, err := NewResultSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewResultSink: %v", err)
		}

		result := step.ChunkResult{
			DocumentID: docID,
			ChunksToSave: []step.ChunkToSave{
				{Text: "list chunk", Position: 0, Type: step.TypeList, ExtractionNodeIDs: []step.ChunkExtractionNodeID{
					{ExtractionNodeID: node1, Position: 0},
					{ExtractionNodeID: node2, Position: 1},
				}},
			},
		}
		if err := sink.SaveChunks(ctx, result); err != nil {
			t.Fatalf("SaveChunks() error = %v", err)
		}

		var chunkID int64
		if err := db.QueryRow(`SELECT id FROM chunks`).Scan(&chunkID); err != nil {
			t.Fatalf("querying chunk id: %v", err)
		}

		rows, err := db.Query(`SELECT chunk_id, extraction_node_id, position FROM chunk_nodes ORDER BY position`)
		if err != nil {
			t.Fatalf("querying chunk_nodes: %v", err)
		}
		defer func() { _ = rows.Close() }()

		type gotNode struct {
			chunkID, extractionNodeID, position int64
		}
		var got []gotNode
		for rows.Next() {
			var n gotNode
			if err := rows.Scan(&n.chunkID, &n.extractionNodeID, &n.position); err != nil {
				t.Fatalf("scanning chunk_nodes row: %v", err)
			}
			got = append(got, n)
		}

		want := []gotNode{
			{chunkID: chunkID, extractionNodeID: node1, position: 0},
			{chunkID: chunkID, extractionNodeID: node2, position: 1},
		}
		if len(got) != len(want) {
			t.Fatalf("got %d chunk_nodes rows, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("chunk_nodes[%d] = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	// Expected to fail red right now, not a test mistake: the FK from
	// chunk_nodes.extraction_node_id to extraction_nodes.id is silently
	// unenforced today because connection.go's DSN has "foreign_keys=ON"
	// where the sqlite3 driver requires "_foreign_keys=ON" (every other
	// pragma param in that same DSN already has the underscore). Confirmed
	// directly: PRAGMA foreign_keys reports 0 against the real DSN, 1 once
	// the underscore is added. Until that's fixed, a chunk_nodes row can
	// silently reference a nonexistent extraction_nodes id with no error at
	// all -- this test documents the correct, intended behavior rather than
	// asserting the current (wrong) one.
	t.Run("a failure partway through rolls back the whole batch, not just the failing chunk", func(t *testing.T) {
		db := sqlitetest.New(t)
		ctx := context.Background()
		docID := sqlitetest.InsertDocument(t, db)
		realNode := sqlitetest.InsertExtractionNode(t, db)

		sink, err := NewResultSink(db, testLogger)
		if err != nil {
			t.Fatalf("NewResultSink: %v", err)
		}

		result := step.ChunkResult{
			DocumentID: docID,
			ChunksToSave: []step.ChunkToSave{
				{Text: "good chunk", Position: 0, Type: step.TypeContent, ExtractionNodeIDs: []step.ChunkExtractionNodeID{
					{ExtractionNodeID: realNode, Position: 0},
				}},
				// references a node id that was never inserted -- violates
				// chunk_nodes' FK to extraction_nodes, which should abort the
				// whole transaction, including the chunk above that already
				// succeeded within it.
				{Text: "bad chunk", Position: 1, Type: step.TypeContent, ExtractionNodeIDs: []step.ChunkExtractionNodeID{
					{ExtractionNodeID: 999999, Position: 0},
				}},
			},
		}
		if err := sink.SaveChunks(ctx, result); err == nil {
			t.Fatalf("SaveChunks() error = nil, want an error for the FK violation on the second chunk")
		}

		var count int
		if err := db.QueryRow(`SELECT count(*) FROM chunks`).Scan(&count); err != nil {
			t.Fatalf("counting chunks: %v", err)
		}
		if count != 0 {
			t.Errorf("got %d chunk rows after a rolled-back batch, want 0 -- the first chunk's insert should not have survived", count)
		}
	})
}
