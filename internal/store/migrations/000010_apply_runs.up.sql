-- apply_runs is the durable record of every apply -- UI, cron loop, and
-- CLI alike. Until now an apply left only a server-side log line and the
-- schedule_history rows it produced, so "why didn't X air last Tuesday"
-- had no answer that survived a restart: the conflict warnings that
-- explain a dropped occurrence were computed on every generate and
-- thrown away with the response.
--
-- A row is written with status 'running' BEFORE the apply touches
-- Tunarr, and finalized afterwards. A row left at 'running' therefore
-- means the process died mid-apply, which is information worth keeping
-- rather than a defect to hide.
--
-- finished_at is nullable for exactly that reason; every other column is
-- NOT NULL with a default so a partially-written run still reads cleanly.
CREATE TABLE apply_runs (
    id            TEXT PRIMARY KEY,
    started_at    TIMESTAMP NOT NULL,
    finished_at   TIMESTAMP,
    source        TEXT NOT NULL,
    scope         TEXT NOT NULL DEFAULT '',
    days          INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL,
    channel_count INTEGER NOT NULL DEFAULT 0,
    slot_count    INTEGER NOT NULL DEFAULT 0,
    error         TEXT NOT NULL DEFAULT ''
);

-- The page reads runs newest-first inside a window; this is the only
-- access pattern.
CREATE INDEX idx_apply_runs_started_at ON apply_runs (started_at DESC);

-- apply_run_warnings persists what scheduler.Warning used to carry only
-- in a response body: one occurrence that was planned a slot and then
-- dropped by conflict resolution. No REFERENCES clause -- foreign keys
-- are not enabled on this database (store.sqliteDSNParams), so a
-- declared cascade would be silently inert; CleanupApplyRuns deletes
-- these rows explicitly instead.
CREATE TABLE apply_run_warnings (
    run_id              TEXT NOT NULL,
    block_name          TEXT NOT NULL,
    occurrence_start    TIMESTAMP NOT NULL,
    blocking_block_name TEXT NOT NULL,
    channel_id          TEXT NOT NULL DEFAULT '',
    duration_minutes    INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_apply_run_warnings_run_id ON apply_run_warnings (run_id);

-- run_id ties an aired programme back to the apply that put it there.
-- Defaulted to '' rather than NULL so rows written before this migration
-- read as "no run recorded" without every consumer handling a NULL --
-- runs cannot be backfilled, which the page's empty state says plainly.
ALTER TABLE schedule_history ADD COLUMN run_id TEXT NOT NULL DEFAULT '';
