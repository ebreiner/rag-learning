CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  name TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  metadata_json TEXT,
  collection_name TEXT NOT NULL,

  FOREIGN KEY(collection_name) REFERENCES collections(name),
  CHECK(metadata_json IS NULL OR json_valid(metadata_json))
);

CREATE TABLE IF NOT EXISTS collections (
  name TEXT PRIMARY KEY,
  weight REAL NOT NULL,

  CHECK(weight >= 0)
);

CREATE TABLE IF NOT EXISTS chunks (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  document_id INTEGER NOT NULL,
  position INTEGER NOT NULL,
  text TEXT NOT NULL,
  breadcrumb TEXT NOT NULL,
  FOREIGN KEY(document_id) REFERENCES documents(id)
);

CREATE TABLE IF NOT EXISTS extractions (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  document_id INTEGER NOT NULL,
  mime_type TEXT NOT NULL,

  FOREIGN KEY(document_id) REFERENCES documents(id)
);

CREATE TABLE IF NOT EXISTS extraction_nodes (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  extraction_id INTEGER NOT NULL,
  node_id TEXT NOT NULL,
  parent_id TEXT,
  kind TEXT NOT NULL,
  layer TEXT NOT NULL,
  content_json TEXT,
  provenance_json TEXT,

  CHECK (content_json IS NULL OR json_valid(content_json)),
  CHECK (provenance_json IS NULL OR json_valid(provenance_json)),
  FOREIGN KEY(extraction_id) REFERENCES extractions(id)
);
