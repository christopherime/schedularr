package schema

// SchedulerFile is the schema for standalone scheduler configuration files
#SchedulerFile: {
	// List of scheduling blocks
	blocks: [...#Block]
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
}
