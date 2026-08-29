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
	// The "showTitle" tag accepts a flat key for fixture/test compat
	// (testdata/programs/*.json, this package's own tests, and any Tunarr
	// deployment that happens to emit a flat showTitle) -- but a *live*
	// Tunarr 1.3.13 instance, live-verified against
	// docs/tunarr/openapi.json's Episode/Show schemas this session, never
	// sends that key at all: a real POST /api/programs/search "episode"
	// result nests show identity under a "show" object instead (see the
	// Show field below), and carries no "rating" of its own -- only Show's
	// does. Client.SearchPrograms and Client.GetFillerPrograms
	// (client.go's hydrateEpisodeShowFields, the single post-unmarshal
	// choke point both call) fill ShowTitle from Show.Title -- and Rating
	// from Show.Rating -- whenever Show is non-nil and the flat field came
	// back empty, so a live response now populates both exactly like a
	// flat-shaped fixture always has. Hydration never overwrites an
	// already-non-empty flat value, so existing fixtures round-trip
	// unchanged. Season/episode numbers have the same flat-vs-nested
	// mismatch (Tunarr nests them under "season"/uses "episodeNumber" only
	// on Episode, no flat "seasonNumber") and are NOT hydrated here -- out
	// of scope for this fix; scheduler.Engine's findEpisode still compares
	// the flat SeasonNumber/EpisodeNumber fields, which a live response
	// still cannot populate.
	ShowTitle string `json:"showTitle,omitempty"`

	// Show carries the nested show object a live Tunarr "episode" search
	// result actually returns (see ShowTitle's doc comment above). Only
	// ever set for Type == "episode" results; nil for movies, tracks, and
	// every flat-shaped fixture that predates this field. Not read
	// directly by any caller today -- hydrateEpisodeShowFields consumes it
	// to populate ShowTitle/Rating, which is what callers actually read.
	Show *Show `json:"show,omitempty"`

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
//
// Field names match the live envelope, live-verified against Tunarr 1.3.13
// this session and corroborated by docs/tunarr/openapi.json: {"results": [...],
// "page": N, "totalPages": N, "totalHits": N, "facetDistribution": {...}}.
// There is no "total" or "limit" key -- a prior version of this struct
// modeled those instead of TotalHits/TotalPages, so they always
// deserialized to zero against a real response, which made every
// resp.Total-based pagination loop (internal/service/schedule.go) stop
// after its first page regardless of how many programs actually matched.
// FacetDistribution is part of the envelope but unused by any caller today,
// so it isn't modeled here.
type ProgramSearchResponse struct {
	Results    []Program `json:"results"`
	Page       int       `json:"page"`
	TotalPages int       `json:"totalPages"`
	TotalHits  int       `json:"totalHits"`
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
