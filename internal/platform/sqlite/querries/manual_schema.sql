CREATE VIRTUAL TABLE IF NOT EXISTS embeddings_balanced_1536 USING vec0(
  chunk_id INTEGER PRIMARY KEY,
  embedding float[768]
);

CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
  text,
  content='chunks',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
