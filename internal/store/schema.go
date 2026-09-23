package store

const sqliteSchema = `
CREATE TABLE components (
  id INTEGER PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  sort_order INTEGER NOT NULL,
  active INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE reports (
  id INTEGER PRIMARY KEY,
  component_id INTEGER NOT NULL REFERENCES components(id),
  category TEXT NOT NULL,
  description TEXT,
  reporter_email TEXT,
  ip_hash TEXT NOT NULL,
  email_status TEXT NOT NULL,
  email_error TEXT,
  created_at INTEGER NOT NULL
);

CREATE TABLE aggregates (
  component_id INTEGER NOT NULL,
  category TEXT NOT NULL,
  bucket_start INTEGER NOT NULL,
  report_count INTEGER NOT NULL,
  PRIMARY KEY (component_id, category, bucket_start)
);

CREATE INDEX idx_agg_bucket ON aggregates(bucket_start);

CREATE TABLE report_windows (
  ip_hash TEXT NOT NULL,
  component_id INTEGER NOT NULL,
  last_at INTEGER NOT NULL,
  PRIMARY KEY (ip_hash, component_id)
);

CREATE TABLE login_attempts (
  ip_hash TEXT NOT NULL PRIMARY KEY,
  attempts INTEGER NOT NULL,
  window_start INTEGER NOT NULL
);
`

const mysqlSchema = `
CREATE TABLE components (
  id BIGINT NOT NULL AUTO_INCREMENT,
  slug VARCHAR(32) NOT NULL,
  name VARCHAR(80) NOT NULL,
  sort_order INT NOT NULL,
  active TINYINT NOT NULL DEFAULT 1,
  PRIMARY KEY (id),
  UNIQUE KEY uq_components_slug (slug)
);

CREATE TABLE reports (
  id BIGINT NOT NULL AUTO_INCREMENT,
  component_id BIGINT NOT NULL,
  category VARCHAR(32) NOT NULL,
  description TEXT NULL,
  reporter_email VARCHAR(254) NULL,
  ip_hash VARCHAR(64) NOT NULL,
  email_status VARCHAR(16) NOT NULL,
  email_error VARCHAR(400) NULL,
  created_at BIGINT NOT NULL,
  PRIMARY KEY (id),
  KEY idx_reports_component (component_id),
  CONSTRAINT fk_reports_component FOREIGN KEY (component_id) REFERENCES components(id)
);

CREATE TABLE aggregates (
  component_id BIGINT NOT NULL,
  category VARCHAR(32) NOT NULL,
  bucket_start BIGINT NOT NULL,
  report_count INT NOT NULL,
  PRIMARY KEY (component_id, category, bucket_start),
  KEY idx_agg_bucket (bucket_start)
);

CREATE TABLE report_windows (
  ip_hash VARCHAR(64) NOT NULL,
  component_id BIGINT NOT NULL,
  last_at BIGINT NOT NULL,
  PRIMARY KEY (ip_hash, component_id)
);

CREATE TABLE login_attempts (
  ip_hash VARCHAR(64) NOT NULL,
  attempts INT NOT NULL,
  window_start BIGINT NOT NULL,
  PRIMARY KEY (ip_hash)
);
`
