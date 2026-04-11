CREATE TABLE documents (
  id INTEGER PRIMARY KEY,
  filename TEXT NOT NULL UNIQUE
);

CREATE TABLE chunks (
  id INTEGER PRIMARY KEY,
  document_id INTEGER NOT NULL,
  chunk_index INTEGER NOT NULL,
  chunk_count INTEGER NOT NULL, -- store count here until chunk meta data table is in place
  text TEXT NOT NULL,
  FOREIGN KEY(document_id) REFERENCES documents(id)
);

