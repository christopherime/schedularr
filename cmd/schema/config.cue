package schema

// Config is the root configuration schema for Schedularr
// This is the single source of truth for configuration structure and defaults
#Config: {
	// Tunarr connection configuration (required)
	tunarr: #TunarrConfig

	// Logging configuration
	log: #LogConfig

	// Path to SQLite database file
	database: string | *"schedularr.db"

	// Path to external scheduler configuration file
	scheduler_file: string | *"scheduler.yaml"

	// Interval between cron-driven schedule regenerate-and-apply cycles
	// (the `serve` command's cron loop). Not under `api` since it governs
	// the scheduler, not the HTTP server -- `serve --interval` overrides
	// this value when explicitly passed.
	cron_interval: string | *"6h"

	// Maintenance configuration for background tasks
	maintenance: #MaintenanceConfig

	// HTTP API server configuration (the `serve` command)
	api: #APIConfig
}

// APIConfig defines the HTTP API server settings used by `schedularr serve`
#APIConfig: {
	// Address the API server listens on (host:port, or :port for all interfaces)
	listen: string | *":8484"

	// Bearer token required on /api/v1/* requests. Also settable via the
	// SCHEDULARR_API_TOKEN environment variable, which always takes
	// precedence over this value. Must be at least 32 characters unless
	// insecure_no_auth is true.
	token: string | *""

	// Skip bearer-token auth on /api/v1/* entirely. Local development only
	// -- never set this in a real deployment.
	insecure_no_auth: bool | *false
}

// TunarrConfig defines the Tunarr API connection settings
#TunarrConfig: {
	// Tunarr API base URL
	url: string | *"${SCHEDULARR_TUNARR_URL}"

	// API key for authentication
	api_key: string | *"${SCHEDULARR_TUNARR_API_KEY}"

	// Request timeout duration
	timeout: string | *"30s"
}

// LogConfig defines logging settings
#LogConfig: {
	// Log level: debug, info, warn, error
	level: "debug" | "info" | "warn" | "error" | *"info"

	// Log format: text or json
	format: "text" | "json" | *"text"

	// IANA Time Zone name (e.g., "America/New_York", "UTC", "Local")
	timezone: string | *"Local"
}

// MaintenanceConfig defines background maintenance task settings
#MaintenanceConfig: {
	// How long to keep schedule history (e.g., "168h" for 7 days). Also
	// governs how much of the persisted schedule_history table GET
	// /history?days=N (api/openapi.yaml, 1..90) can actually return -- see
	// internal/service/schedule.go's NewRunner doc comment.
	history_retention: string | *"168h"

	// Whether to enable automatic cleanup
	cleanup_enabled: bool | *true
}

// Block defines a scheduling block (filter-based or series-based)
#Block: {
	// Block type: "filter" or "series"
	type: "filter" | "series" | *"filter"

	// Human-readable block name
	name: string

	// Cron expression (standard 5-field format: minute hour dom month dow)
	cron: string

	// Block duration in minutes
	duration: int & >0

	// Target Tunarr channel ID
	channel_id: string

	// Priority for conflict resolution (higher = more important)
	priority: int | *10

	// Max minutes a block's actual duration can exceed its planned duration
	max_duration_overflow_minutes: int | *0

	// Filter criteria (for type="filter")
	filter?: #Filter

	// Series configuration (for type="series")
	series?: [...#SeriesConfig]

	// Fallback configuration when series completes
	fallback?: #FallbackConfig

	// Filler content configuration
	filler?: #FillerConfig
}

// Filter defines content filtering criteria
#Filter: {
	// Title pattern (regex)
	title_pattern?: string

	// Genre filter (any match)
	genres?: [...string]

	// Rating filter (any match)
	ratings?: [...string]

	// Year range filter
	year_from?: int
	year_to?:   int

	// Duration range in minutes
	min_duration?: int
	max_duration?: int

	// Tag filter
	tags?: [...string]
}

// SeriesConfig defines a series for sequential scheduling
#SeriesConfig: {
	// Show title to match
	show_title: string

	// Number of episodes per block
	episodes_per_block: int & >0 | *1

	// Starting season (optional, defaults to 1)
	start_season: int & >0 | *1

	// Starting episode (optional, defaults to 1)
	start_episode: int & >0 | *1

	// Action when series completes: "continue", "restart", or "disable"
	on_complete: "continue" | "restart" | "disable" | *"continue"

	// Episodes to skip (format: "S01E05", "S02E10")
	skip_episodes?: [...string]

	// Maximum number of times to run through series (0 = unlimited)
	max_runs: int & >=0 | *0
}

// FallbackConfig defines fallback behavior when series completes
#FallbackConfig: {
	// Fallback mode: "redistribute" or "filler"
	mode: "redistribute" | "filler" | *"redistribute"

	// Filler filter (used when mode="filler")
	filler_filter?: #Filter
}

// FillerConfig defines filler content configuration for a block
#FillerConfig: {
	// Whether to use filler content
	enabled: bool | *false

	// ID of filler list to use
	filler_list_id?: string

	// Max minutes of filler allowed (0 = unlimited)
	max_filler_time: int | *0

	// Minimum gap (minutes) before adding filler
	min_gap_time: int | *5
}

// Default configuration instance - all defaults come from type definitions
config: #Config
