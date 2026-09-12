-- Rebuild with the pre-v0.5.13 PRIMARY KEY. Fails if the data holds the
-- legitimate repeats the new identity allows -- that is the constraint
-- this migration exists to remove.
CREATE TABLE schedule_history_old (
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
  show_title TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (program_id, channel_id, scheduled_at)
);

INSERT INTO schedule_history_old (program_id, channel_id, block_name, scheduled_at, occurrence_start, sequence, duration_ms, title, type, run_id, show_title)
SELECT program_id, channel_id, block_name, scheduled_at, occurrence_start, sequence, duration_ms, title, type, run_id, show_title
FROM schedule_history;

DROP TABLE schedule_history;
ALTER TABLE schedule_history_old RENAME TO schedule_history;

CREATE INDEX idx_schedule_history_recent ON schedule_history (channel_id, scheduled_at);
CREATE INDEX idx_schedule_history_show ON schedule_history (show_title);
CREATE INDEX idx_schedule_history_occurrence ON schedule_history (block_name, occurrence_start);
