-- name: CreateDocument :one
INSERT INTO documents (
	created_at,
	name,
	sha256,
	metadata_json
) VALUES (
	?,?,?,?
)
RETURNING id;

-- name: ExistsDocument :one
SELECT  id, sha256
FROM documents
WHERE sha256 = ?;


-- name: InsertChunk :exec
INSERT INTO chunks (
	document_id,
	position,
	text,
	breadcrumb,
	created_at
) VALUES (?,?,?,?,?);

-- name: DocAlreadyChunked :one
SELECT id
FROM chunks
WHERE document_id = ?
LIMIT 1;

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
SELECT c.id, d.name, c.position, c.text
FROM chunks AS c
JOIN documents AS d
	ON c.document_id = d.id
WHERE c.id IN (sqlc.slice('chunk_ids'));

