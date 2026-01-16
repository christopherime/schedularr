package schema

// SchedulerFile is the schema for standalone scheduler configuration files
#SchedulerFile: {
	// List of scheduling blocks
	blocks: [...#Block]

	// Optional global settings
	settings?: #SchedulerSettings
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
