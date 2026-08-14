CREATE TABLE IF NOT EXISTS documents (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  name TEXT NOT NULL,
  sha256 TEXT NOT NULL,
  metadata_json TEXT,

  CHECK(metadata_json IS NULL OR json_valid(metadata_json))
);

CREATE TABLE IF NOT EXISTS representations (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  document_id INTEGER NOT NULL,
  parent_representation_id INTEGER,
  stage TEXT NOT NULL,
  FOREIGN KEY(document_id) REFERENCES documents(id),
  FOREIGN KEY(parent_representation_id) REFERENCES representations(id)
);

CREATE TABLE IF NOT EXISTS chunks (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  representation_id INTEGER NOT NULL,
  position INTEGER NOT NULL,
  text TEXT NOT NULL,
  breadcrumb TEXT NOT NULL,
  FOREIGN KEY(representation_id) REFERENCES representations(id)
);

CREATE TABLE IF NOT EXISTS extractions (
  id INTEGER PRIMARY KEY,
  created_at DATETIME NOT NULL,
  representation_id INTEGER NOT NULL,
  mime_type TEXT NOT NULL,

  FOREIGN KEY(representation_id) REFERENCES representations(id)
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

CREATE TABLE IF NOT EXISTS golden_queries (
	id INTEGER PRIMARY KEY,
	created_at DATETIME NOT NULL,
	query_text TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS golden_expectations (
	id INTEGER PRIMARY KEY,
	created_at DATETIME NOT NULL,
	golden_query_id INTEGER NOT NULL,
	document_name TEXT NOT NULL,
	content_sha256 TEXT NOT NULL,
	breadcrumb_contains TEXT,
	text_contains TEXT,

	FOREIGN KEY(golden_query_id) REFERENCES golden_queries(id)
);

CREATE TABLE IF NOT EXISTS eval_runs (
	id INTEGER PRIMARY KEY,
	run_at DATETIME NOT NULL,
	comment TEXT
);

CREATE TABLE IF NOT EXISTS eval_results (
	id INTEGER PRIMARY KEY,
	eval_run_id INTEGER NOT NULL,
	golden_expectation_id INTEGER NOT NULL,
	strategy TEXT NOT NULL,
	matched BOOL NOT NULL,

	FOREIGN KEY(eval_run_id) REFERENCES eval_runs(id),
	FOREIGN KEY(golden_expectation_id) REFERENCES golden_expectations(id)
);

