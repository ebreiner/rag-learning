CREATE VIRTUAL TABLE embeddings_balanced_768 USING vec0(
  chunk_id INTEGER PRIMARY KEY,
  embedding float[768]
);

CREATE VIRTUAL TABLE chunks_fts USING fts5(
  text,
  content='chunks',
  content_rowid='id',
  tokenize='unicode61 remove_diacritics 2'
);
