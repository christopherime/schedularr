DROP INDEX IF EXISTS idx_schedule_history_occurrence;
ALTER TABLE schedule_history DROP COLUMN type;
ALTER TABLE schedule_history DROP COLUMN title;
ALTER TABLE schedule_history DROP COLUMN duration_ms;
ALTER TABLE schedule_history DROP COLUMN sequence;
ALTER TABLE schedule_history DROP COLUMN occurrence_start;
