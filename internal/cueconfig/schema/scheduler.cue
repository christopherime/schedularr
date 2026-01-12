package schema

// SchedulerFile is the schema for standalone scheduler configuration files
#SchedulerFile: {
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

	// Action when series completes: "continue", "restart", or "disable"
	on_complete?: "continue" | "restart" | "disable" | *"continue"

	// Episodes to skip (format: "S01E05", "S02E10")
	skip_episodes?: [...string]

	// Maximum number of times to run through series (0 = unlimited)
	max_runs?: int & >=0 | *0
}

// FallbackConfig defines fallback behavior when series completes
#FallbackConfig: {
	// Fallback mode: "redistribute" or "filler"
	mode: "redistribute" | "filler" | *"redistribute"

	// Filler filter (used when mode="filler")
	filler_filter?: #Filter
}

// Example scheduler configuration with defaults
scheduler: #SchedulerFile & {
	blocks: [
		{
			type:       "filter"
			name:       "Morning Cartoons"
			cron:       "0 6 * * *"
			duration:   240
			channel_id: "channel-1"
			priority:   10
			filter: {
				genres:       ["Animation", "Family"]
				max_duration: 30
				ratings:      ["TV-Y", "TV-G"]
				year_from:    2000
			}
		},
	]
	settings: {
		rotation_window_days: 7
		min_gap_minutes:      5
		max_filler_minutes:   30
	}
}
