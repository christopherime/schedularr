package scheduler

// Filter defines criteria for selecting content
type Filter struct {
	TitlePattern string   `mapstructure:"title_pattern" yaml:"title_pattern"`
	Genres       []string `mapstructure:"genres" yaml:"genres"`
	Ratings      []string `mapstructure:"ratings" yaml:"ratings"`
	YearFrom     int      `mapstructure:"year_from" yaml:"year_from"`
	YearTo       int      `mapstructure:"year_to" yaml:"year_to"`
	MinDuration  int      `mapstructure:"min_duration" yaml:"min_duration"` // in minutes
	MaxDuration  int      `mapstructure:"max_duration" yaml:"max_duration"` // in minutes
	Tags         []string `mapstructure:"tags" yaml:"tags"`
}

// Block defines a scheduled programming block
type Block struct {
	Name      string `mapstructure:"name" yaml:"name"`
	Cron      string `mapstructure:"cron" yaml:"cron"`         // Cron expression for start time
	Duration  int    `mapstructure:"duration" yaml:"duration"` // Duration in minutes
	Filter    Filter `mapstructure:"filter" yaml:"filter"`
	ChannelID string `mapstructure:"channel_id" yaml:"channel_id"`
	Priority  int    `mapstructure:"priority" yaml:"priority"` // Higher priority overrides overlapping blocks
}

// Config holds the scheduling configuration
type Config struct {
	Blocks []Block `mapstructure:"blocks" yaml:"blocks"`
}
