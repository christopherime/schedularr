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
	// Duration is float64, not int64: live-verified this session that a
	// real Tunarr instance's channel.duration (server-computed as the sum
	// of its lineup items' own -- also float -- durations, see
	// LineupRepository.ts's updateChannel) can come back with a fractional
	// millisecond component (e.g. 691200000.9999), matching
	// tunarr.Program.Duration's existing "float from Tunarr API"
	// convention. An int64 field here used to make GetChannels's response
	// decode fail on any such channel -- which, worse, is not a one-shot
	// failure: internal/httpclient.Client's resty retry condition fires on
	// any non-nil error regardless of HTTP status (see
	// httpclient.DefaultConfig's AddRetryCondition), so a decode error
	// alone triggered up to MaxRetries (3) extra live requests against
	// Tunarr for a response shape that could never decode differently.
	// This field is otherwise unused by any caller in this repo (grepped),
	// so widening its type has no other effect.
	Duration float64 `json:"duration"`
	Stealth  bool    `json:"stealth"`
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

	// Type identifies what kind of entry this is. The oneof below is the
	// full set of values a live /api/programs/search result can carry --
	// live-verified and audited against docs/tunarr/openapi.json's
	// per-item type enum this session (a prior round's list was missing
	// "season", which is exactly the pre-existing bug that made a
	// growing library's scheduled scan discard every accumulated page
	// once season-type entries appeared: validateProgram errored on the
	// first one, and the caller treated that as a hard fetch failure --
	// see Client.SearchPrograms/GetFillerPrograms's doc comments for the
	// skip-and-continue fix, which is the actual defense against any
	// value still missing from this list). "redirect"/"custom"/"flex"/
	// "content" are not search-result types at all -- they're Tunarr
	// channel-programming slot types (see ChannelProgram.Type), kept
	// here because Program doubles as UpdateSchedule's request body
	// shape.
	Type string `json:"type" validate:"omitempty,oneof=movie episode track music_video other_video redirect custom flex content show season album artist collection folder playlist"`
	// Subtype is used in filler content: movie, episode, track, music_video, other_video
	Subtype string `json:"subtype,omitempty"`

	// Index is a Type == "season" (or "show") entry's own 1-based
	// ordinal -- live-verified this session: a search result's
	// season-type entry carries `"index": N` directly (the same wire key
	// GET /api/programming/seasons/{id} uses -- see Season.SeasonNumber's
	// doc comment). service.Runner.hydrateSeasonNumbers reads this to
	// build a local SeasonID -> index map from whatever season entries
	// already showed up in the accumulated result set, before falling
	// back to Client.GetSeason for any SeasonID that didn't.
	Index int `json:"index,omitempty"`

	// ShowID and SeasonID are the foreign keys a live Tunarr "episode"
	// search result actually carries -- live-verified against a real
	// Tunarr 1.3.13 instance this session (transcript in this task's
	// report; a prior round of this fix claimed, from a spec read alone,
	// that episodes nest a "show" object instead -- that claim was wrong,
	// see ShowTitle's doc comment below). ShowID points at a separate
	// Type == "show" Program entry (matched by that entry's own
	// UUID/ID, via GetID()) that a live search interleaves in the SAME
	// paginated result stream as episodes, not nested inside them.
	// SeasonID points at a Type == "season" entry the same way -- also
	// interleaved in the same result stream (live-verified: a 100-item
	// page can be entirely season entries), just not necessarily
	// co-located with the episodes that reference it, and not guaranteed
	// to appear in every fetch at all -- see hydrateShowsAndSeasons' doc
	// comment in schedule.go for why the season-index resolver
	// (Client.GetSeason) remains a fallback for whichever SeasonIDs the
	// local join (via Index above) doesn't cover.
	// service.Runner.hydrateShowsAndSeasons (internal/service/schedule.go)
	// is the only production consumer of both: it joins ShowID against
	// the accumulated result set's Type == "show" entries to fill
	// ShowTitle/Rating, and resolves each distinct SeasonID -- first
	// against any interleaved Type == "season" entry's Index, then via
	// Client.GetSeason -- to fill SeasonNumber.
	ShowID   string `json:"showId,omitempty"`
	SeasonID string `json:"seasonId,omitempty"`
	// SeasonNumber is not sent as a flat field by a live episode result at
	// all (unlike EpisodeNumber, which is) -- it stays 0 until
	// service.Runner.hydrateSeasonNumbers resolves it via SeasonID (see
	// above). The "seasonNumber" tag exists for fixture/test compat only
	// (testdata/programs/*.json, this package's own tests), matching
	// ShowTitle's flat-tag-for-fixtures/join-for-live split below.
	SeasonNumber  int `json:"seasonNumber,omitempty"`
	EpisodeNumber int `json:"episodeNumber,omitempty"`

	// ShowTitle identifies which show a Type == "episode" program belongs
	// to -- the field scheduler.Engine's series matching
	// (findEpisode/planSeriesForConfig, internal/scheduler/engine.go) and
	// internal/service.Runner.MediaShows both group/match on.
	//
	// The "showTitle" tag accepts a flat key for fixture/test compat
	// (testdata/programs/*.json, this package's own tests, and any Tunarr
	// deployment that happens to emit a flat showTitle) -- but a *live*
	// Tunarr 1.3.13 instance never sends that key, and never nests a
	// "show" object under an episode either (see the Show field below --
	// live-verified this session with an actual captured response; a
	// prior round of this fix modeled that nested shape from a spec read
	// alone and was wrong: 0 of 84 live episodes captured carried one).
	// What a live episode actually carries is the ShowID FK above. The
	// PRODUCTION path that fills ShowTitle (and Rating, which an episode
	// also carries no value of its own for) against live data is
	// service.Runner.hydrateShowTitleAndRating, which joins ShowID against
	// separate Type == "show" entries Tunarr interleaves in the same
	// search result stream -- see ShowID's doc comment above and
	// hydrateShowsAndSeasons's doc comment in
	// internal/service/schedule.go for the full mechanism and the live
	// evidence for why this must be a post-pagination, whole-result-set
	// join rather than a per-page client-side one.
	//
	// Show/hydrateEpisodeShowFields (client.go) hydrate this the same way
	// (fill only if empty) from a nested "show" object, and are kept as a
	// harmless secondary path -- correct if some future/richer Tunarr
	// response ever does nest show data -- but do not fire against Tunarr
	// 1.3.13 today. Both hydration paths, and the flat tag itself, only
	// ever fill an already-empty field, so a flat-shaped fixture always
	// round-trips unchanged regardless of which path (if any) also ran.
	ShowTitle string `json:"showTitle,omitempty"`

	// Show would carry a nested show object if a live Tunarr "episode"
	// search result ever actually returned one -- it does not, against
	// Tunarr 1.3.13 (live-verified this session; see ShowTitle's doc
	// comment above for what a live episode carries instead, and for why
	// this field is being kept anyway). Always nil against real data
	// today; only ever non-nil when a caller constructs one directly
	// (this package's own hydration tests). Not read directly by any
	// production caller -- hydrateEpisodeShowFields (client.go) consumes
	// it defensively to populate ShowTitle/Rating, which is what callers
	// actually read, in case a richer response shape ever does nest it.
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

// Season represents a TV show season. Retrieved individually from GET
// /api/programming/seasons/{id} (see Client.GetSeason) -- the only way to
// learn an episode's season number against a live Tunarr instance, since
// neither a search result's episode entry nor any interleaved entry in
// that same result carries it (see tunarr.Program.SeasonID's doc comment).
type Season struct {
	UUID  string `json:"uuid"`
	Title string `json:"title,omitempty"`
	// SeasonNumber is the season's 1-based ordinal (e.g. 1 for "Season
	// 1"). The wire key is "index", not "seasonNumber" -- live-verified
	// against Tunarr 1.3.13 (GET /api/programming/seasons/{id} response)
	// and corroborated by docs/tunarr/openapi.json; a prior version of
	// this struct used the wrong ("seasonNumber") key and so never
	// actually deserialized this field from a real response.
	SeasonNumber int `json:"index"`
	ChildCount   int `json:"childCount"` // Episode count
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
//
// There used to be a Filter *SearchFilter field here, modeled as
// {Type []string}. No code path in this repo ever constructed one (grepped
// every SearchFilter{...} literal and every ProgramSearchQuery{...}
// literal in this task's report -- always empty/omitted). It was also
// simply wrong: the real request schema's query.filter is
// SearchFilterInput, an expression-tree shape ({type: "op", op: "or"/
// "and", children: [...]} or {type: "value", fieldSpec: {...}, op, value}),
// nothing like a flat type list -- live-verified this session: POSTing our
// old {"filter": {"type": [...]}} shape against a live instance returns
// HTTP 400 FST_ERR_VALIDATION ("body/query/filter/type Invalid input").
// Removed entirely (no-legacy) rather than fixed, since nothing used it;
// if a caller needs server-side filtering in the future, restrictSearchTo
// (below) already models the simpler, correctly-typed alternative Tunarr
// also accepts for narrowing by content type.
type ProgramSearchQuery struct {
	Query            string      `json:"query,omitempty"`
	RestrictSearchTo []string    `json:"restrictSearchTo,omitempty"`
	Sort             *SearchSort `json:"sort,omitempty"`
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

	// DroppedCount is not part of the wire response (json:"-") -- it's
	// Client.SearchPrograms' own bookkeeping, set to how many entries this
	// page's Results originally contained that failed validateProgram and
	// were dropped rather than aborting the whole page (see
	// SearchPrograms' doc comment for why: one weird/unrecognized entry
	// must never discard an entire fetch). Callers that paginate
	// (internal/service/schedule.go's fetchSingleLibrary/
	// fetchAllProgramsViaSearch) accumulate this across every page and
	// log one summary WARN per whole fetch, not per page.
	DroppedCount int `json:"-"`
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

// ManualLineupRequest is the request body Client.UpdateSchedule sends to
// POST /api/channels/{id}/programming -- the "manual" variant of a
// discriminated union of three lineup-update kinds Tunarr also accepts
// ("manual", "time", "random"; only "manual" is modeled here, since it's
// the only one any caller in this repo constructs).
//
// Source-verified against tag v1.3.13 (github.com/chrisbenincasa/tunarr):
// types/src/api/index.ts's ManualProgramLineupSchema --
//
//	z.object({ type: z.literal('manual'), lineup: CondensedChannelProgramSchema.array(), append: z.boolean().default(false) })
//
// -- and server/src/db/channel/LineupRepository.ts's updateLineup, whose
// "manual" branch (`if (req.type === 'manual')`) reads only req.lineup,
// never req.programs. This deliberately does NOT match the vendored
// docs/tunarr/openapi.json (v1.0.16), which lists this variant as also
// requiring a top-level "programs" field -- that document is stale for
// this route: live-verified this session that a real Tunarr 1.3.13
// instance has no PUT route for this path at all (404 "Route PUT:...
// not found"), which the vendored spec doesn't reflect either.
type ManualLineupRequest struct {
	Type   string       `json:"type"` // always "manual"
	Lineup []LineupItem `json:"lineup"`
	Append bool         `json:"append"`
}

// LineupItem is one entry of a ManualLineupRequest's "lineup" array.
// Schedularr sends two of CondensedChannelProgramSchema's five variants:
//
//   - "content": a movie/episode/track Tunarr already knows about, from
//     SearchPrograms/library results. ID must be an existing Tunarr
//     program UUID (see Program.GetID, which prefers UUID over the legacy
//     ID field): updateLineup's channelProgramToLineupItemFunc
//     (LineupRepository.ts) carries a content entry's "id" straight
//     through to the channel_programs.program_uuid foreign key, so an ID
//     Tunarr doesn't already recognize produces a broken lineup entry
//     rather than a validation error -- there is no server-side existence
//     check at this layer.
//   - "flex": dead-air/offline padding, no ID. Source-verified,
//     types/src/schemas/lineupPrograms.ts's FlexProgramSchema:
//     `BaseProgramSchema.extend({type: z.literal('flex'), fillerConfig:
//     OfflineFillerConfigSchema.optional()})` -- fillerConfig is always
//     omitted here (falls back to Tunarr's own configured flex behavior).
//     Duration must be strictly positive for a flex entry
//     (BaseProgramSchema.duration is `z.number().positive()`, unlike
//     content's `z.number().min(0)`) -- see
//     service.buildAnchoredLineup's zero-gap guard, the only lineup
//     builder in this repo, which never emits a zero-duration flex entry.
type LineupItem struct {
	Type     string  `json:"type"` // "content" or "flex"
	ID       string  `json:"id,omitempty"`
	Duration float64 `json:"duration"` // milliseconds
}
