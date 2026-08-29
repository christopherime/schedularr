DROP TABLE IF EXISTS series_occurrence_snapshots;

CREATE TABLE IF NOT EXISTS series_occurrence_snapshots (
  block_name       TEXT NOT NULL,
  occurrence_start DATETIME NOT NULL,
  snapshot_json    TEXT NOT NULL,
  PRIMARY KEY (block_name, occurrence_start)
);
