-- name: GetChunksForIDs :many
SELECT c.id, c.chunk_count, c.text, c.chunk_index, c.document_id, d.filename FROM chunks AS c
	JOIN documents AS d
	ON c.document_id = d.id
	WHERE c.id IN (sqlc.slice('ids'));

