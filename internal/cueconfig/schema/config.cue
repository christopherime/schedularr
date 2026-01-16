package schema

// Config is the root configuration schema for Schedularr
#Config: {
	// Tunarr connection configuration
	tunarr: #TunarrConfig

	// Radarr connection configuration
	radarr: #RadarrConfig

	// Sonarr connection configuration
	sonarr: #SonarrConfig

	// Jellyfin connection configuration
	jellyfin: #JellyfinConfig

	// Logging configuration
	log: #LogConfig

	// Port for Prometheus metrics endpoint
	metrics_port: int | *9090

	// Path to SQLite database file
	database: string | *"schedularr.db"

	// Path to external scheduler configuration file
	scheduler_file: string | *"scheduler.yaml"

	// Inline scheduler configuration (legacy support)
	scheduler?: #SchedulerConfig

	// Content caching configuration
	cache: #CacheConfig

	// Maintenance configuration for background tasks
	maintenance: #MaintenanceConfig
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

// CacheConfig defines content caching settings
#CacheConfig: {
	// Directory to store cached content metadata
	cache_dir: string | *"/tmp/schedularr_cache" @tag(go, "filepath.Join(os.TempDir(), \"schedularr_cache\")")

	// How long cached entries are considered valid (e.g., "1h", "24h")
	cache_duration: string | *"1h"
}

// RadarrConfig defines the Radarr API connection settings
#RadarrConfig: {
	// Radarr API base URL
	url: string | *"${SCHEDULARR_RADARR_URL}"

	// API key for authentication
	api_key: string | *"${SCHEDULARR_RADARR_API_KEY}"

	// Exclude movies that are missing files on disk
	exclude_missing_file: bool | *true
}

// SonarrConfig defines the Sonarr API connection settings
#SonarrConfig: {
	// Sonarr API base URL
	url: string | *"${SCHEDULARR_SONARR_URL}"

	// API key for authentication
	api_key: string | *"${SCHEDULARR_SONARR_API_KEY}"

	// Exclude episodes that are missing files on disk
	exclude_missing_file: bool | *true
}

// JellyfinConfig defines the Jellyfin API connection settings
#JellyfinConfig: {
	// Jellyfin API base URL
	url: string | *"${SCHEDULARR_JELLYFIN_URL}"

	// API key for authentication
	api_key: string | *"${SCHEDULARR_JELLYFIN_API_KEY}"

	// User ID for user-scoped endpoints
	user_id: string | *""

	// Whether to refresh the Live TV guide after schedule apply
	sync_live_tv: bool | *true
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
	// How often to run cleanup tasks (e.g., "24h")
	cleanup_interval: string | *"24h"

	// How long to keep schedule history (e.g., "168h" for 7 days)
	history_retention: string | *"168h"

	// Whether to enable automatic cleanup
	cleanup_enabled: bool | *true
}

// SchedulerConfig defines the scheduling configuration
#SchedulerConfig: {
	// List of scheduling blocks
	blocks: [...#Block]

	// Optional global settings
	settings?: #SchedulerSettings
}

// SchedulerSettings defines global scheduler settings
#SchedulerSettings: {
	// Default rotation window in days (prevent re-scheduling within X days)
	rotation_window_days?: int | *7

	// Minimum gap time in minutes to trigger filler content
	min_gap_minutes?: int | *5

	// Maximum filler duration in minutes
	max_filler_minutes?: int | *30
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

	// Filter criteria (for type="filter")
	filter?: #Filter

	// Series configuration (for type="series")
	series?: [...#SeriesConfig]

	// Fallback configuration when series completes
	fallback?: #FallbackConfig
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
	start_season?: int & >0 | *1

	// Starting episode (optional, defaults to 1)
	start_episode?: int & >0 | *1
}

// FallbackConfig defines fallback behavior when series completes
#FallbackConfig: {
	// Fallback mode: "redistribute" or "filler"
	mode: "redistribute" | "filler" | *"redistribute"

	// Filler filter (used when mode="filler")
	filler_filter?: #Filter
}

// Default configuration instance - all defaults come from type definitions
config: #Config
