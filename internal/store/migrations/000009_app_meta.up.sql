-- app_meta is a one-table key/value store for app-level facts that are
-- not rows of any domain table. Its first key, last_apply_at, records the
-- instant the most recent apply pushed at least one lineup to Tunarr
-- (planned pushes and stale-channel clears alike -- pushing a flex-only
-- lineup IS an apply).
--
-- Persisted independently of applied_channels on purpose: that table is a
-- TRACKING SET, not a log -- clearStaleChannels removes a row again the
-- moment a stale channel has been cleared, so its max(applied_at) can
-- vanish (single channel cleared -> empty set -> "never applied") or go
-- backwards (the newest row cleared -> an older row becomes the max)
-- seconds after a successful apply. GET /status's last_applied_at needs a
-- value that only ever moves forward, which is what this key provides
-- (v0.5.0 bench-rebuild review, MAJOR-1).
CREATE TABLE app_meta (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
