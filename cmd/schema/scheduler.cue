package scheduler

#Filter: {
	title_pattern?: string
	genres?: [...string]
	ratings?: [...string]
	year_from?: int
	year_to?: int
	min_duration?: int
	max_duration?: int
	tags?: [...string]
}

#FillerConfig: {
	enabled: bool | *false
	filler_list_id?: string
	max_filler_time?: int
	min_gap_time?: int
}

#SeriesConfig: {
	show_title: string
	episodes_per_block: int & >0
	start_season?: int
	start_episode?: int
}

#SeriesFallback: {
	mode: "redistribute" | "filler" | "none" | *""
	filler_filter?: #Filter
}

#Block: {
	type: "filter" | "series" | *"filter"
	name: string
	cron: string
	duration: int & >0
	channel_id: string
	priority: int | *0
	
	// Filter Block specific
	filter?: #Filter
	filler?: #FillerConfig

	// Series Block specific
	series?: [...#SeriesConfig]
	fallback?: #SeriesFallback
}

blocks: [...#Block]
