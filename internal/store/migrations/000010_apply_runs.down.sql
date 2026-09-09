DROP INDEX IF EXISTS idx_apply_run_warnings_run_id;
DROP TABLE IF EXISTS apply_run_warnings;
DROP INDEX IF EXISTS idx_apply_runs_started_at;
DROP TABLE IF EXISTS apply_runs;
-- DROP COLUMN needs SQLite >= 3.35; mattn/go-sqlite3 bundles well past it.
ALTER TABLE schedule_history DROP COLUMN run_id;
