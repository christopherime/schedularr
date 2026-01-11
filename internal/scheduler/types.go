package scheduler

// BlockType defines the type of scheduling block
type BlockType string

const (
	// BlockTypeFilter schedules content based on filter criteria
	BlockTypeFilter BlockType = "filter"
	// BlockTypeSeries schedules specific series in sequential order
	BlockTypeSeries BlockType = "series"
)

// FallbackMode defines how to handle empty time in a series block
type FallbackMode string

const (
	// FallbackModeRedistribute redistributes remaining time to other series
	FallbackModeRedistribute FallbackMode = "redistribute"
	// FallbackModeFiller fills remaining time with content matching FillerFilter
	FallbackModeFiller FallbackMode = "filler"
)

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

// FillerConfig defines filler content configuration for a block
type FillerConfig struct {
	Enabled       bool   `mapstructure:"enabled" yaml:"enabled"`             // Whether to use filler content
	FillerListID  string `mapstructure:"filler_list_id" yaml:"filler_list_id"` // ID of filler list to use
	MaxFillerTime int    `mapstructure:"max_filler_time" yaml:"max_filler_time"` // Max minutes of filler allowed (0 = unlimited)
	MinGapTime    int    `mapstructure:"min_gap_time" yaml:"min_gap_time"`    // Minimum gap (minutes) before adding filler
}

// SeriesConfig defines configuration for a specific series in a block
type SeriesConfig struct {
	ShowTitle        string `mapstructure:"show_title" yaml:"show_title"`
	EpisodesPerBlock int    `mapstructure:"episodes_per_block" yaml:"episodes_per_block"`
	StartSeason      int    `mapstructure:"start_season" yaml:"start_season"`     // Optional: override start point
	StartEpisode     int    `mapstructure:"start_episode" yaml:"start_episode"`   // Optional: override start point
}

// SeriesFallback defines fallback behavior when series content runs out or doesn't fill duration
type SeriesFallback struct {
	Mode         FallbackMode `mapstructure:"mode" yaml:"mode"`
	FillerFilter Filter       `mapstructure:"filler_filter" yaml:"filler_filter"` // Used if Mode is Filler
}

// Block defines a scheduled programming block
type Block struct {
	Type      BlockType      `mapstructure:"type" yaml:"type"`         // "filter" or "series", default "filter"
	Name      string         `mapstructure:"name" yaml:"name"`
	Cron      string         `mapstructure:"cron" yaml:"cron"`         // Cron expression for start time
	Duration  int            `mapstructure:"duration" yaml:"duration"` // Duration in minutes
	Filter    Filter         `mapstructure:"filter" yaml:"filter"`
	ChannelID string         `mapstructure:"channel_id" yaml:"channel_id"`
	Priority  int            `mapstructure:"priority" yaml:"priority"` // Higher priority overrides overlapping blocks
	Filler    FillerConfig   `mapstructure:"filler" yaml:"filler"`     // Filler content configuration
	Series    []SeriesConfig `mapstructure:"series" yaml:"series"`     // For BlockTypeSeries
	Fallback  SeriesFallback `mapstructure:"fallback" yaml:"fallback"` // For BlockTypeSeries
}

// Config holds the scheduling configuration
type Config struct {
	Blocks []Block `mapstructure:"blocks" yaml:"blocks"`
}
