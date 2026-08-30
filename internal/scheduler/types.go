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
	TitlePattern string   `mapstructure:"title_pattern" yaml:"title_pattern" json:"title_pattern,omitempty"`
	Genres       []string `mapstructure:"genres" yaml:"genres" json:"genres,omitempty"`
	Ratings      []string `mapstructure:"ratings" yaml:"ratings" json:"ratings,omitempty"`
	YearFrom     int      `mapstructure:"year_from" yaml:"year_from" json:"year_from,omitempty"`
	YearTo       int      `mapstructure:"year_to" yaml:"year_to" json:"year_to,omitempty"`
	MinDuration  int      `mapstructure:"min_duration" yaml:"min_duration" json:"min_duration,omitempty"` // in minutes
	MaxDuration  int      `mapstructure:"max_duration" yaml:"max_duration" json:"max_duration,omitempty"` // in minutes
	Tags         []string `mapstructure:"tags" yaml:"tags" json:"tags,omitempty"`
}

// FillerConfig defines filler content configuration for a block
type FillerConfig struct {
	Enabled       bool   `mapstructure:"enabled" yaml:"enabled" json:"enabled"`                                   // Whether to use filler content
	FillerListID  string `mapstructure:"filler_list_id" yaml:"filler_list_id" json:"filler_list_id,omitempty"`    // ID of filler list to use
	MaxFillerTime int    `mapstructure:"max_filler_time" yaml:"max_filler_time" json:"max_filler_time,omitempty"` // Max minutes of filler allowed (0 = unlimited)
	MinGapTime    int    `mapstructure:"min_gap_time" yaml:"min_gap_time" json:"min_gap_time,omitempty"`          // Minimum gap (minutes) before adding filler
}

// CompletionAction defines what to do when a series completes all episodes
type CompletionAction string

const (
	// CompletionActionContinue keeps the series in the block but marks it complete (default)
	CompletionActionContinue CompletionAction = "continue"
	// CompletionActionRestart restarts the series from the beginning
	CompletionActionRestart CompletionAction = "restart"
	// CompletionActionDisable removes the series from future scheduling
	CompletionActionDisable CompletionAction = "disable"
)

// SeriesConfig defines configuration for a specific series in a block
type SeriesConfig struct {
	ShowTitle        string           `mapstructure:"show_title" yaml:"show_title" json:"show_title"`
	EpisodesPerBlock int              `mapstructure:"episodes_per_block" yaml:"episodes_per_block" json:"episodes_per_block"`
	StartSeason      int              `mapstructure:"start_season" yaml:"start_season" json:"start_season,omitempty"`    // Optional: override start point
	StartEpisode     int              `mapstructure:"start_episode" yaml:"start_episode" json:"start_episode,omitempty"` // Optional: override start point
	OnComplete       CompletionAction `mapstructure:"on_complete" yaml:"on_complete" json:"on_complete,omitempty"`       // Action when series completes (default: continue)
	SkipEpisodes     []string         `mapstructure:"skip_episodes" yaml:"skip_episodes" json:"skip_episodes,omitempty"` // Episodes to skip (format: "S01E05", "S02E10")
	MaxRuns          int              `mapstructure:"max_runs" yaml:"max_runs" json:"max_runs,omitempty"`                // Max times to run through series (0 = unlimited)
}

// SeriesFallback defines fallback behavior when series content runs out or doesn't fill duration
type SeriesFallback struct {
	Mode         FallbackMode `mapstructure:"mode" yaml:"mode" json:"mode,omitempty"`
	FillerFilter Filter       `mapstructure:"filler_filter" yaml:"filler_filter" json:"filler_filter,omitempty"` // Used if Mode is Filler
}

// Block defines a scheduled programming block
type Block struct {
	// ID is the block's stable store identity (store.BlockRecord.ID, a
	// UUID) -- populated by service.ActiveBlocks from the store record
	// this Spec came from, since scheduler.yaml-imported/test-constructed
	// Blocks have no such identity of their own. Unlike Name (unique, but
	// renameable), ID never changes across a block's lifetime, which is
	// what keys series_occurrence_snapshots by (see
	// Engine.planSeriesOccurrences and StateStore.GetOccurrenceSnapshot's
	// doc comments) rather than Name -- a rename would otherwise orphan
	// every not-yet-aired occurrence's stored cursor snapshot. May be
	// empty for a Block never sourced from the store (e.g. some tests) --
	// callers that key off it must tolerate that.
	ID                         string         `mapstructure:"-" yaml:"-" json:"-"`
	Type                       BlockType      `mapstructure:"type" yaml:"type,omitempty" json:"type"` // "filter" or "series", default "filter"; omitempty so a zero value re-renders as absent and CUE's *"filter" default applies instead of "" failing the disjunction
	Name                       string         `mapstructure:"name" yaml:"name" json:"name"`
	Cron                       string         `mapstructure:"cron" yaml:"cron" json:"cron"`             // Cron expression for start time
	Duration                   int            `mapstructure:"duration" yaml:"duration" json:"duration"` // Duration in minutes
	Filter                     Filter         `mapstructure:"filter" yaml:"filter" json:"filter,omitempty"`
	ChannelID                  string         `mapstructure:"channel_id" yaml:"channel_id" json:"channel_id"`
	Priority                   int            `mapstructure:"priority" yaml:"priority" json:"priority"`                                                                          // Higher priority overrides overlapping blocks
	MaxDurationOverflowMinutes int            `mapstructure:"max_duration_overflow_minutes" yaml:"max_duration_overflow_minutes" json:"max_duration_overflow_minutes,omitempty"` // Max minutes a block's actual duration can exceed its planned duration
	Filler                     FillerConfig   `mapstructure:"filler" yaml:"filler" json:"filler,omitempty"`                                                                      // Filler content configuration
	Series                     []SeriesConfig `mapstructure:"series" yaml:"series" json:"series,omitempty"`                                                                      // For BlockTypeSeries
	Fallback                   SeriesFallback `mapstructure:"fallback" yaml:"fallback,omitempty" json:"fallback,omitempty"`                                                      // For BlockTypeSeries
}

// Config holds the scheduling configuration
type Config struct {
	Blocks []Block `mapstructure:"blocks" yaml:"blocks" json:"blocks"`
}
