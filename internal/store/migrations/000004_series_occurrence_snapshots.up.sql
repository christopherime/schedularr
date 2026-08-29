-- Per-occurrence series cursor snapshots: the per-show season/episode
-- (and completed/disabled/run_count) state a series block's occurrence
-- was first planned from, captured immutably. See
-- scheduler.SeriesStateSnapshot's and scheduler.Engine.PlanBlock's doc
-- comments for the mechanism this supports -- idempotent re-apply of a
-- not-yet-aired occurrence that still lets a block spec edited before it
-- airs (series reordered, added, removed, episodes_per_block or duration
-- changed) change that occurrence's content.
CREATE TABLE IF NOT EXISTS series_occurrence_snapshots (
  block_name       TEXT NOT NULL,
  occurrence_start DATETIME NOT NULL,
  snapshot_json    TEXT NOT NULL,
  PRIMARY KEY (block_name, occurrence_start)
);
