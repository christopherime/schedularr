-- Replace schedule_history's original PRIMARY KEY (program_id,
-- channel_id, scheduled_at) with the identity the occurrence model
-- (migration 3) actually gave a row: (block_name, occurrence_start,
-- sequence).
--
-- The old key predates occurrences and misfires two ways, both fatal to
-- an apply against a small library: scheduled_at is wall-clock PLANNING
-- time, identical (to the second) for every occurrence planned in one
-- apply, so the same program legitimately planned for two occurrences of
-- a multi-day window on one channel collides; and the same program
-- repeated within a single long occurrence collides even inside one
-- slot. Neither is a duplicate -- the engine's recency dedup is a
-- planning heuristic, not an integrity rule.
--
-- SQLite cannot drop a PK in place, so this is a table rebuild. The new
-- integrity rule is a PARTIAL unique index: rows with no occurrence
-- identity are exempt -- pre-migration-3 rows share the epoch
-- occurrence_start default ('1970-01-01 00:00:00'), and a row written
-- with Go's zero time stores '0001-01-01 ...'. Both sort below
-- '1971-01-01' as text in every format this store has ever written,
-- and both are already invisible to the occurrence replay lookup.
CREATE TABLE schedule_history_rebuilt (
  program_id TEXT NOT NULL,
  channel_id TEXT NOT NULL,
  block_name TEXT NOT NULL,
  scheduled_at DATETIME NOT NULL,
  occurrence_start DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00',
  sequence INTEGER NOT NULL DEFAULT 0,
  duration_ms REAL NOT NULL DEFAULT 0,
  title TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  show_title TEXT NOT NULL DEFAULT ''
);

INSERT INTO schedule_history_rebuilt (program_id, channel_id, block_name, scheduled_at, occurrence_start, sequence, duration_ms, title, type, run_id, show_title)
SELECT program_id, channel_id, block_name, scheduled_at, occurrence_start, sequence, duration_ms, title, type, run_id, show_title
FROM schedule_history;

DROP TABLE schedule_history;
ALTER TABLE schedule_history_rebuilt RENAME TO schedule_history;

CREATE INDEX idx_schedule_history_recent ON schedule_history (channel_id, scheduled_at);
CREATE INDEX idx_schedule_history_show ON schedule_history (show_title);
CREATE UNIQUE INDEX idx_schedule_history_occurrence ON schedule_history (block_name, occurrence_start, sequence)
  WHERE occurrence_start > '1971-01-01';
