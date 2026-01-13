package schema

// Config is the root configuration schema for Schedularr
#Config: {
	// Tunarr connection configuration
	tunarr: #TunarrConfig

	// Optional Radarr connection configuration
	radarr?: #RadarrConfig

	// Optional Sonarr connection configuration
	sonarr?: #SonarrConfig

	// Optional Jellyfin connection configuration
	jellyfin?: #JellyfinConfig

	// Logging configuration
	log: #LogConfig

	// Optional path to SQLite database file (defaults to ~/.schedularr.db)
	database?: string

	// Optional path to external scheduler configuration file
	scheduler_file?: string

	// Inline scheduler configuration (legacy support)
	scheduler?: #SchedulerConfig

	// Optional content caching configuration
	cache: #CacheConfig
}

// TunarrConfig defines the Tunarr API connection settings
#TunarrConfig: {
	// Tunarr API base URL
	url: string | *"http://localhost:8000"

	// Optional API key for authentication
	api_key?: string

	// Request timeout duration
	timeout?: string | *"10s"
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
	url: string

	// Optional API key for authentication
	api_key?: string

	// Exclude movies that are missing files on disk
	exclude_missing_file?: bool | *true
}

// SonarrConfig defines the Sonarr API connection settings
#SonarrConfig: {
	// Sonarr API base URL
	url: string

	// Optional API key for authentication
	api_key?: string

	// Exclude episodes that are missing files on disk
	exclude_missing_file?: bool | *true
}

// JellyfinConfig defines the Jellyfin API connection settings
#JellyfinConfig: {
	// Jellyfin API base URL
	url: string

	// Optional API key for authentication
	api_key?: string

	// Optional user ID for user-scoped endpoints
	user_id?: string

	// Whether to refresh the Live TV guide after schedule apply
	sync_live_tv?: bool | *false
}

// LogConfig defines logging settings
#LogConfig: {
	// Log level: debug, info, warn, error
	level: "debug" | "info" | "warn" | "error" | *"info"

	// Log format: text or json
	format: "text" | "json" | *"text"
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

// Default configuration instance
config: #Config & {
	tunarr: {
		url: "http://localhost:8000"
	}
	log: {
		level:  "info"
		format: "text"
	}
	cache: {
		cache_dir:      "/tmp/schedularr_cache"
		cache_duration: "1h"
	}
}
