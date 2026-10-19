-- name: CreateDocument :one
INSERT INTO documents (
	created_at,
	name,
	rel_path,
	sha256,
	source_name,
	metadata_json
) VALUES (
	?,?,?,?,?,?
)
RETURNING id;

-- name: ExistsDocument :one
SELECT id, sha256, source_name
FROM documents
WHERE sha256 = ?;


-- name: SaveSource :exec
INSERT INTO sources (
	name,
	weight
) VALUES (?,?)
ON CONFLICT(name) DO
UPDATE SET weight = excluded.weight;


-- name: InsertChunk :one
INSERT INTO chunks (
	document_id,
	position,
	type,
	text,
	breadcrumb,
	created_at
) VALUES (?,?,?,?,?,?)
RETURNING id;

-- name: InsertChunkNodes :exec
INSERT INTO chunk_nodes(
	chunk_id,
	extraction_node_id,
	created_at,
	position
) VALUES(?,?,?,?);

-- name: DocAlreadyChunked :one
SELECT id
FROM chunks
WHERE document_id = ?
LIMIT 1;

-- name: FlushChunkNodes :exec
DELETE FROM chunk_nodes;

-- name: FlushChunks :exec
DELETE FROM chunks;

-- name: CreateExtraction :one
INSERT INTO extractions (
	document_id,
	created_at,
	mime_type
) VALUES (
	?,?,?
)
RETURNING id;


-- name: CreateExtractionNode :exec
INSERT INTO extraction_nodes (
      extraction_id,
      created_at,
      node_id,
      parent_id,
      kind,
      layer,
      content_json,
      provenance_json
) VALUES (
      ?,?,?,?,?,?,?,?
);


-- name: GetDocumentIDsAfterID :one
SELECT id
FROM documents
WHERE  id > ?
ORDER BY id;


-- name: GetLatestExtractionOfDoc :many
SELECT
	e.id AS extraction_id,
	e.document_id,
	e.mime_type,
	en.id AS extraction_node_id,
	en.node_id,
	en.parent_id,
	en.kind,
	en.layer,
	en.provenance_json,
	en.content_json
FROM extractions e
JOIN extraction_nodes en
	ON en.extraction_id = e.id
WHERE e.document_id = ?
ORDER BY en.id;


-- name: RetrievalChunksByIDs :many
SELECT
	c.id,
	d.name,
	d.source_name,
	s.weight,
	c.position,
	c.breadcrumb,
	c.text,
	d.metadata_json
FROM chunks AS c
JOIN documents AS d
	ON c.document_id = d.id
JOIN sources AS s
	ON d.source_name = s.name
WHERE c.id IN (sqlc.slice('chunk_ids'));

-- name: EvalDocExists :one
SELECT EXISTS(SELECT 1 FROM documents WHERE name = ?);

-- name: EvalChunksOfDoc :many
SELECT c.id, c.breadcrumb, c.text, d.name
FROM chunks AS c
JOIN documents AS d ON c.document_id = d.id
WHERE d.name = ?;

-- name: EvalChunkStats :one
SELECT
	COUNT(*) AS chunk_count,
	CAST(COALESCE(MAX(created_at), '') AS TEXT) AS latest_created_at
FROM chunks;
