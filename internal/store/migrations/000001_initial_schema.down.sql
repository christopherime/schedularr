-- Rollback initial schema
DROP INDEX IF EXISTS idx_schedule_history_recent;
DROP TABLE IF EXISTS schedule_history;
DROP TABLE IF EXISTS series_state;
