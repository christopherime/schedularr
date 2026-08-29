package tunarr

// ChannelIcon represents a channel icon configuration.
type ChannelIcon struct {
	Path     string `json:"path"`
	Width    int    `json:"width"`
	Duration int    `json:"duration"`
	Position string `json:"position"` // top-left, top-right, bottom-left, bottom-right
}

// Channel represents a Tunarr TV channel.
type Channel struct {
	ID         string       `json:"id" validate:"required"`
	Number     int          `json:"number"`
	Name       string       `json:"name" validate:"required"`
	Icon       *ChannelIcon `json:"icon,omitempty"`
	GroupTitle string       `json:"groupTitle"`
	Duration   int64        `json:"duration"`
	Stealth    bool         `json:"stealth"`
}

// Genre represents a genre with metadata.
type Genre struct {
	UUID string `json:"uuid,omitempty"`
	Name string `json:"name"`
}

// Program represents a media program (movie, episode, or track) in Tunarr.
// Field names match the Tunarr API schema.
type Program struct {
	// Core identifier - Tunarr uses "id" for legacy/filler content
	ID string `json:"id,omitempty"`
	// UUID is the primary identifier in newer API responses
	UUID string `json:"uuid,omitempty"`

	Title    string  `json:"title" validate:"required"`
	Year     *int    `json:"year,omitempty"`
	Summary  string  `json:"summary,omitempty"`
	Duration float64 `json:"duration" validate:"gte=0"` // in milliseconds (float from Tunarr API, 0 for placeholders)
	Rating   string  `json:"rating,omitempty"`
	Icon     string  `json:"icon,omitempty"`

	// Genres can be simple strings or objects depending on the endpoint
	Genres []Genre `json:"genres,omitempty"`

	// Type: movie, episode, track, music_video, other_video, redirect, custom, flex, show (Tunarr may return others)
	Type string `json:"type" validate:"omitempty,oneof=movie episode track music_video other_video redirect custom flex content show"`
	// Subtype is used in filler content: movie, episode, track, music_video, other_video
	Subtype string `json:"subtype,omitempty"`

	// Episode-specific fields (Tunarr uses seasonNumber/episodeNumber)
	ShowID        string `json:"showId,omitempty"`
	SeasonID      string `json:"seasonId,omitempty"`
	SeasonNumber  int    `json:"seasonNumber,omitempty"`
	EpisodeNumber int    `json:"episodeNumber,omitempty"`

	// ShowTitle identifies which show an Type == "episode" program belongs
	// to -- the field scheduler.Engine's series matching
	// (findEpisode/planSeriesForConfig, internal/scheduler/engine.go) and
	// internal/service.Runner.MediaShows both group/match on.
	//
	// This field's tag used to be `json:"-"`, meaning encoding/json
	// silently dropped it on every marshal *and* unmarshal -- so a real
	// SearchPrograms HTTP response could never populate it, and neither
	// could a fake test server round-tripping a tunarr.Program value
	// through actual JSON (only a Go struct literal, bypassing JSON
	// entirely, could ever produce a non-empty ShowTitle). That gap was
	// previously documented and deliberately left alone in
	// internal/service/schedule_test.go
	// (TestRunner_Run_ChannelScopedApply_LeavesOtherChannelStateUntouched's
	// doc comment) for a task where it didn't matter; it directly blocks
	// internal/service.Runner.MediaShows, whose entire job is grouping by
	// this field, so this task fixes the tag to accept a flat "showTitle"
	// key -- matching testdata/programs/*.json and this package's own
	// README-documented fixture format.
	//
	// This does not fully match a *live* Tunarr instance: per
	// docs/tunarr/openapi.json (the vendored Tunarr 1.0.16 OpenAPI spec),
	// a real /api/programs/search "episode" result nests show data under a
	// "show" object ({"show": {"title": ..., "uuid": ..., "rating": ...,
	// "genres": ...}}), not a flat "showTitle" string -- and episode
	// entries carry no "rating" of their own at all, only via that nested
	// show. Modeling that nested shape (and updating every ShowTitle
	// caller's assumptions, notably the season/episode numbers below,
	// which have the same flat-vs-nested mismatch) is a larger client
	// change with its own blast radius across scheduler.Engine, out of
	// scope here; see this task's report for the full field-by-field
	// evidence. This fix only widens what already-flat-shaped input (this
	// package's own test fixtures, and any Tunarr deployment that happens
	// to emit a flat showTitle) deserializes into -- it does not change
	// behavior for a payload that omits the key entirely, which is what a
	// nested-shape real response looks like from this struct's point of
	// view today.
	ShowTitle string `json:"showTitle,omitempty"`

	// Source information
	SourceType    string `json:"sourceType,omitempty"` // plex, jellyfin, emby, local
	MediaSourceID string `json:"mediaSourceId,omitempty"`
	LibraryID     string `json:"libraryId,omitempty"`
	ExternalID    string `json:"externalId,omitempty"`
	ExternalKey   string `json:"externalKey,omitempty"`
}

// GetID returns the program ID, preferring UUID over ID.
func (p *Program) GetID() string {
	if p.UUID != "" {
		return p.UUID
	}
	return p.ID
}

// GetYear returns the year value or 0 if nil.
func (p *Program) GetYear() int {
	if p.Year != nil {
		return *p.Year
	}
	return 0
}

// GetDurationMs returns the duration in milliseconds as int64.
// Tunarr API returns duration as float64 for precision.
func (p *Program) GetDurationMs() int64 {
	return int64(p.Duration)
}

// GetGenreNames returns a slice of genre names for filtering.
func (p *Program) GetGenreNames() []string {
	names := make([]string, 0, len(p.Genres))
	for _, g := range p.Genres {
		if g.Name != "" {
			names = append(names, g.Name)
		}
	}
	return names
}

// Library represents a media library from a media source.
// Retrieved from /api/media-sources/{id}/libraries
type Library struct {
	ID            string `json:"id" validate:"required"`
	Name          string `json:"name" validate:"required"`
	MediaType     string `json:"mediaType"` // movies, shows, music_videos, other_videos, tracks
	Type          string `json:"type"`      // plex, jellyfin, emby, local
	Enabled       bool   `json:"enabled"`
	ExternalKey   string `json:"externalKey,omitempty"`
	LastScannedAt int64  `json:"lastScannedAt,omitempty"`
}

// Season represents a TV show season.
type Season struct {
	UUID         string `json:"uuid"`
	Title        string `json:"title,omitempty"`
	SeasonNumber int    `json:"seasonNumber"`
	ChildCount   int    `json:"childCount"` // Episode count
}

// Show represents a TV show with metadata.
type Show struct {
	UUID    string `json:"uuid" validate:"required"`
	Title   string `json:"title" validate:"required"`
	Year    *int   `json:"year,omitempty"`
	Summary string `json:"summary,omitempty"`
	Rating  string `json:"rating,omitempty"`

	// Genres is an array of Genre objects
	Genres []Genre `json:"genres,omitempty"`

	// Season/episode counts
	ChildCount      int `json:"childCount"`      // Number of seasons
	GrandchildCount int `json:"grandchildCount"` // Number of episodes

	// Seasons contains the actual season data
	Seasons []Season `json:"seasons,omitempty"`

	// Source information
	SourceType    string `json:"sourceType,omitempty"` // plex, jellyfin, emby, local
	MediaSourceID string `json:"mediaSourceId,omitempty"`
	LibraryID     string `json:"libraryId,omitempty"`
	ExternalID    string `json:"externalId,omitempty"`
}

// FillerList represents a collection of filler content.
type FillerList struct {
	ID           string    `json:"id" validate:"required"`
	Name         string    `json:"name" validate:"required"`
	ContentCount int       `json:"contentCount"` // number of items
	Programs     []Program `json:"programs,omitempty"`
}

// MediaSource represents a connected media server (Plex/Jellyfin/Emby).
type MediaSource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"` // plex, jellyfin, emby, local
}

// ProgramSearchQuery represents a search query for programs.
type ProgramSearchQuery struct {
	Query            string        `json:"query,omitempty"`
	RestrictSearchTo []string      `json:"restrictSearchTo,omitempty"`
	Filter           *SearchFilter `json:"filter,omitempty"`
	Sort             *SearchSort   `json:"sort,omitempty"`
}

// SearchFilter represents search filter options.
type SearchFilter struct {
	Type []string `json:"type,omitempty"` // movie, episode, track, etc.
}

// SearchSort represents search sort options.
type SearchSort struct {
	Field     string `json:"field,omitempty"`     // title, year, duration, etc.
	Direction string `json:"direction,omitempty"` // asc, desc
}

// ProgramSearchRequest represents the request body for POST /api/programs/search.
type ProgramSearchRequest struct {
	Query         *ProgramSearchQuery `json:"query,omitempty"`
	MediaSourceID string              `json:"mediaSourceId,omitempty"`
	LibraryID     string              `json:"libraryId,omitempty"`
	Page          int                 `json:"page,omitempty"`
	Limit         int                 `json:"limit,omitempty"`
}

// ProgramSearchResponse represents the response from POST /api/programs/search.
type ProgramSearchResponse struct {
	Results []Program `json:"results"`
	Total   int       `json:"total"`
	Page    int       `json:"page"`
	Limit   int       `json:"limit"`
}

// ScheduleSlot represents a time slot in a channel schedule.
type ScheduleSlot struct {
	StartTime int64  `json:"startTime"` // milliseconds from start of period
	Type      string `json:"type"`      // movie, show, flex, redirect
	Order     string `json:"order"`     // next, shuffle, ordered_shuffle, alphanumeric, chronological
	Direction string `json:"direction"` // asc, desc
}

// TimeBasedSchedule represents a time-based channel schedule.
type TimeBasedSchedule struct {
	Type           string         `json:"type"` // "time"
	FlexPreference string         `json:"flexPreference,omitempty"`
	LatenessMs     int64          `json:"latenessMs,omitempty"`
	MaxDays        int            `json:"maxDays,omitempty"`
	PadMs          int64          `json:"padMs,omitempty"`
	Period         string         `json:"period,omitempty"` // day, week
	Slots          []ScheduleSlot `json:"slots,omitempty"`
}

// ScheduleRequest represents the request body for POST /api/channels/{id}/schedule-time-slots.
type ScheduleRequest struct {
	Schedule *TimeBasedSchedule `json:"schedule"`
}
