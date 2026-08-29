-- Extends schedule_history with per-occurrence assignment data so an
-- apply is idempotent: a previously committed occurrence (block_name,
-- occurrence_start) is replayed from these columns instead of being
-- re-planned, which used to re-advance series cursors and duplicate
-- history on every apply that still covered that occurrence's time in
-- its window. See scheduler.Engine.PlanBlock's doc comment.
--
-- occurrence_start/sequence/duration_ms/title/type all default to a
-- value that can never match a real lookup (epoch for occurrence_start,
-- zero/empty for the rest), so pre-migration rows are simply invisible
-- to the new occurrence lookup rather than colliding with it.
ALTER TABLE schedule_history ADD COLUMN occurrence_start DATETIME NOT NULL DEFAULT '1970-01-01 00:00:00';
ALTER TABLE schedule_history ADD COLUMN sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE schedule_history ADD COLUMN duration_ms REAL NOT NULL DEFAULT 0;
ALTER TABLE schedule_history ADD COLUMN title TEXT NOT NULL DEFAULT '';
ALTER TABLE schedule_history ADD COLUMN type TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_schedule_history_occurrence ON schedule_history (block_name, occurrence_start);
