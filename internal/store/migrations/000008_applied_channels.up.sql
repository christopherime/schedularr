-- applied_channels tracks which Tunarr channels the most recent applies
-- pushed a lineup to. It exists so an apply can clear (push a flex-only
-- lineup to) a channel whose blocks have all since been deleted or
-- disabled: without it, a channel that drops out of the plan simply stops
-- being pushed to at all, and whatever lineup the last apply left behind
-- keeps airing in Tunarr indefinitely. A channel is untracked again as
-- soon as its stale lineup has been cleared once, so a channel the
-- operator takes over manually in Tunarr afterwards is never clobbered on
-- every subsequent apply -- see service.Runner.clearStaleChannels.
CREATE TABLE applied_channels (
    channel_id TEXT PRIMARY KEY,
    applied_at TIMESTAMP NOT NULL
);

-- Seed from schedule_history: every channel schedularr scheduled content
-- on in the past is a channel a previous (pre-migration) apply pushed a
-- lineup to, so pre-existing deployments get stale-lineup clearing for
-- channels that were already orphaned before this table existed.
INSERT INTO applied_channels (channel_id, applied_at)
SELECT channel_id, MAX(scheduled_at)
FROM schedule_history
GROUP BY channel_id;
