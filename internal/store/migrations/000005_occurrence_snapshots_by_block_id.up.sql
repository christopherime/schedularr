-- Re-keys series_occurrence_snapshots from block_name to block_id (the
-- block's stable store UUID, blocks.id). block_name is renameable via
-- PATCH/PUT on a block, which used to orphan every not-yet-aired
-- occurrence's stored cursor snapshot on a rename -- see
-- scheduler.Block.ID's and StateStore.GetOccurrenceSnapshot's doc
-- comments.
--
-- Snapshots are scratch/derived data, not source of truth: losing one
-- across this migration just means the next apply treats that
-- not-yet-aired occurrence as unseen again and re-derives it from
-- current series_state/block spec, which is safe (identical to what
-- DeleteFutureOccurrenceSnapshots does deliberately elsewhere). So this
-- drops and recreates rather than attempting a data-preserving rekey
-- that would require joining against blocks.name, which isn't even
-- unique over time (a deleted-and-recreated block can reuse a name).
DROP TABLE IF EXISTS series_occurrence_snapshots;

-- recorded_at is when this row was last written (real wall-clock time),
-- refreshed on every upsert -- distinct from occurrence_start, which is
-- the (potentially far future OR, for test fixtures, far past-looking)
-- calendar time the occurrence itself airs at. CleanupOccurrenceSnapshots
-- prunes by recorded_at, mirroring CleanupScheduleHistory's own cutoff
-- (schedule_history.scheduled_at, also a real-wall-clock write
-- timestamp) rather than occurrence_start, so retention means "written
-- more than window ago," not "airs more than window from now" -- the
-- latter would prune a legitimately just-created snapshot outright
-- whenever occurrence_start itself is far from the real clock (e.g. a
-- test fixture, or any occurrence generated for a schedule window that
-- starts somewhere other than "now").
CREATE TABLE IF NOT EXISTS series_occurrence_snapshots (
  block_id         TEXT NOT NULL,
  occurrence_start DATETIME NOT NULL,
  snapshot_json    TEXT NOT NULL,
  recorded_at      DATETIME NOT NULL,
  PRIMARY KEY (block_id, occurrence_start)
);
