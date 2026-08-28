-- Blocks move from scheduler.yaml to the store (API-editable source of truth)
CREATE TABLE IF NOT EXISTS blocks (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL UNIQUE,
  enabled    INTEGER NOT NULL DEFAULT 1,
  spec_json  TEXT NOT NULL,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
