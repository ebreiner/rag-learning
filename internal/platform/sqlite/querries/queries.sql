-- TODO: in pakete auftrennen

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


-- name: CreateRepresentation :one
INSERT INTO representations (
	document_id,
	parent_representation_id,
	stage,
	created_at
) VALUES (
	?,?,?,?
)
RETURNING id;


-- name: InsertChunk :exec
INSERT INTO chunks (
	representation_id,
	position,
	text,
	breadcrumb,
	created_at
) VALUES (?,?,?,?,?);


-- name: CreateExtraction :one
INSERT INTO extractions (
	representation_id,
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


-- name: GetChunkBatchAfterID :many
SELECT id, text
FROM chunks
WHERE embedded = 0
	AND id > ?
ORDER BY id
LIMIT ?;


-- name: CountChunks :one
SELECT count(id)
FROM chunks;


-- name: HeadChunks :many
SELECT *
FROM chunks
ORDER BY id
LIMIT ?;


-- name: CountDocuments :one
SELECT count(id)
FROM documents;


-- name: HeadDocuments :many
SELECT *
FROM documents
ORDER BY id
LIMIT ?;


-- name: CountRepresentations :one
SELECT count(id)
FROM representations;


-- name: HeadRepresentations :many
SELECT *
FROM representations
ORDER BY id
LIMIT ?;


-- name: CountExtractions :one
SELECT count(id)
FROM extractions;


-- name: HeadExtractions :many
SELECT *
FROM extractions
ORDER BY id
LIMIT ?;


-- name: CountExtractionNodes :one
SELECT count(id)
FROM extraction_nodes;


-- name: HeadExtractionNodes :many
SELECT *
FROM extraction_nodes
ORDER BY id
LIMIT ?;


-- name: GetDocumentIDsAfterID :one
SELECT id
FROM documents
WHERE  id > ?
ORDER BY id;


-- name: GetLatestExtractionOfDoc :many
WITH latest_extraction AS (
  SELECT
    r.id AS representation_id,
    r.created_at AS representation_created_at,
    e.id AS extraction_id,
    e.mime_type
  FROM representations r
  JOIN extractions e
    ON e.representation_id = r.id
  WHERE r.document_id = ?
    AND r.stage = 'extract'
  ORDER BY r.created_at DESC, r.id DESC
  LIMIT 1
)
SELECT
  le.representation_id,
  le.representation_created_at,
  le.mime_type,
  en.*
FROM latest_extraction le
JOIN extraction_nodes en
  ON en.extraction_id = le.extraction_id
ORDER BY en.id;


-- name: CreateChildRepresentationFromParent :one
INSERT INTO representations (
  document_id,
  parent_representation_id,
  stage,
  created_at
)
SELECT
  r.document_id,
  ?,
  ?,
  ?
FROM representations r
WHERE r.id = ?
RETURNING representations.id;


-- name: RetrievalChunksByIDs :many
SELECT c.id, d.name, c.position, c.text
FROM chunks AS c
JOIN representations r ON r.id = c.representation_id
JOIN documents d ON d.id = r.document_id
WHERE c.id IN (sqlc.slice('chunk_ids'));

