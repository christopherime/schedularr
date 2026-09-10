-- The `title` column has always held the EPISODE title (program.Title),
-- so nothing in this schema identified a SHOW's airings -- which is why
-- "remove everything from Bloody Mary" had no predicate to run against.
--
-- Empty is a real value here, not a missing one: a movie belongs to no
-- show, and a removal by show title must match on a non-empty value so it
-- can never sweep every showless airing along with its target.
--
-- Rows written before this migration keep the default and cannot be
-- backfilled. Recovering a show from a program_id needs the live Tunarr
-- catalog, which may no longer carry that program at all, and guessing
-- from the block's spec is wrong for any block scheduling more than one
-- show. Documented rather than worked around.
ALTER TABLE schedule_history ADD COLUMN show_title TEXT NOT NULL DEFAULT '';

-- Removal filters on this column; retention still filters on scheduled_at
-- via idx_schedule_history_recent, so this is an addition rather than a
-- replacement.
CREATE INDEX IF NOT EXISTS idx_schedule_history_show ON schedule_history (show_title);
