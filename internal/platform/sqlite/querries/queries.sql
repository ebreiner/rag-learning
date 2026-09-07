-- name: CreateDocument :one
INSERT INTO documents (
	created_at,
	name,
	sha256,
	collection_name,
	metadata_json
) VALUES (
	?,?,?,?,?
)
RETURNING id;

-- name: ExistsDocument :one
SELECT  d.id, d.sha256, c.name AS collection_name
FROM documents d
JOIN collections c
ON d.collection_name = c.name
WHERE d.sha256 = ?;


-- name: CreateCollectionOrUpdateWeight :exec
INSERT INTO collections (
	name,
	weight
) VALUES (?,?)
ON CONFLICT DO
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


-- name: GetAllCollectionWeights :many
SELECT name, weight FROM collections;

-- name: RetrievalChunksByIDs :many
SELECT c.id, d.name, d.collection_name, c.position, c.breadcrumb, c.text
FROM chunks AS c
JOIN documents AS d
	ON c.document_id = d.id
WHERE c.id IN (sqlc.slice('chunk_ids'));

-- name: CollectionWeightForChunkIDs :many
SELECT chunks.id, collections.weight
FROM chunks
JOIN documents
ON chunks.document_id = documents.id
JOIN collections
ON documents.collection_name = collections.name
WHERE chunks.id IN (sqlc.slice('chunk_ids'));

