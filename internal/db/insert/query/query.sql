-- name: CreateDocument :one
INSERT INTO documents (
	filename
) VALUES (
	?
)
RETURNING id;

-- name: InsertChunk :exec
INSERT INTO chunks (
	id,
	document_id,
	chunk_index,
	chunk_count,
	text
) VALUES (?,?,?,?,?);

