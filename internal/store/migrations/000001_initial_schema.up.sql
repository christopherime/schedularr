-- Initial schema for Schedularr
-- Creates tables for series state tracking and schedule history
CREATE TABLE IF NOT EXISTS series_state (
  show_title TEXT PRIMARY KEY,
  current_season INTEGER NOT NULL DEFAULT 1,
  current_episode INTEGER NOT NULL DEFAULT 1,
  completed BOOLEAN NOT NULL DEFAULT 0,
  last_aired DATETIME,
  run_count INTEGER NOT NULL DEFAULT 0,
  disabled BOOLEAN NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS schedule_history (
  program_id TEXT NOT NULL,
  channel_id TEXT NOT NULL,
  block_name TEXT NOT NULL,
  scheduled_at DATETIME NOT NULL,
  PRIMARY KEY (program_id, channel_id, scheduled_at)
);
CREATE INDEX IF NOT EXISTS idx_schedule_history_recent ON schedule_history (channel_id, scheduled_at);
