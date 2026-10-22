CREATE TABLE IF NOT EXISTS sources (
  name TEXT PRIMARY KEY,
  weight REAL NOT NULL,

  CHECK(weight > 0)
);

CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  name TEXT NOT NULL,
  rel_path TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  metadata_json TEXT,
  source_name TEXT NOT NULL,

  FOREIGN KEY(source_name) REFERENCES sources(name),
  UNIQUE(source_name, rel_path),
  CHECK(metadata_json IS NULL OR json_valid(metadata_json))
);

CREATE TABLE IF NOT EXISTS chunks (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  document_id INTEGER NOT NULL,
  position INTEGER NOT NULL,
  type TEXT NOT NULL,
  text TEXT NOT NULL,
  breadcrumb TEXT NOT NULL,
  FOREIGN KEY(document_id) REFERENCES documents(id)
);

CREATE TABLE IF NOT EXISTS chunk_nodes (
  chunk_id INTEGER NOT NULL,
  extraction_node_id INTEGER NOT NULL,
  created_at DATETIME NOT NULL,
  position INTEGER NOT NULL,

  FOREIGN KEY(chunk_id) REFERENCES chunks(id),
  FOREIGN KEY(extraction_node_id) REFERENCES extraction_nodes(id)
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

CREATE TABLE IF NOT EXISTS document_links (
  from_document_id INTEGER NOT NULL,
  position INTEGER NOT NULL,
  to_source TEXT NOT NULL,
  to_rel_path TEXT NOT NULL,
  text TEXT,
  section TEXT NOT NULL DEFAULT '',

  PRIMARY KEY (from_document_id, position),
  FOREIGN KEY (from_document_id) REFERENCES documents(id) ON DELETE CASCADE,
  UNIQUE (from_document_id, to_source, to_rel_path, section)
);
CREATE INDEX IF NOT EXISTS document_links_target ON document_links (to_source, to_rel_path);
