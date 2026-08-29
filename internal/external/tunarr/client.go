// Package tunarr provides a client for interacting with the Tunarr API.
package tunarr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/christopherime/schedularr/internal/httpclient"
	"github.com/christopherime/schedularr/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

// Client is a Tunarr API client.
type Client struct {
	http *httpclient.Client
}

// NewClient creates a new Tunarr API client with the given configuration.
func NewClient(cfg Config) *Client {
	httpCfg := httpclient.DefaultConfig(
		strings.TrimRight(cfg.URL, "/"),
		cfg.APIKey,
		httpclient.AuthXAPIKey,
	)
	return &Client{
		http: httpclient.New(httpCfg),
	}
}

// validateChannel validates a Channel struct using struct tags.
func validateChannel(ch *Channel) error {
	return httpclient.Validate(ch)
}

// validateProgram validates a Program struct using struct tags.
func validateProgram(p *Program) error {
	return httpclient.Validate(p)
}

// hydrateEpisodeShowFields is a defensive, secondary hydration path run at
// the post-unmarshal choke point every []Program-decoding client method
// (SearchPrograms, GetFillerPrograms) runs its results through. It fills
// ShowTitle/Rating from a nested Program.Show object when present -- but
// against a real Tunarr 1.3.13 instance, Program.Show is always nil (see
// Program.ShowTitle's doc comment in models.go: a live episode result
// carries no nested show object at all, only a ShowID foreign key). This
// function therefore does not fire in production today; it is kept
// because it's correct and harmless (fill-only-if-empty) for a
// richer/future response shape that did nest show data, and because a
// caller/test double is free to construct one directly. The PRODUCTION
// path that fills ShowTitle/Rating against live data is
// service.Runner.hydrateShowTitleAndRating
// (internal/service/schedule.go), which joins an episode's ShowID against
// separate Type == "show" entries Tunarr interleaves in the same search
// result stream -- necessarily a post-pagination step over a whole
// accumulated result set (see that function's doc comment for why), which
// this single-page, per-response choke point cannot do.
//
// Hydration only ever fills an empty field -- it never overwrites a
// non-empty flat value a caller/fixture already supplied -- and only
// touches Type == "episode" programs with a non-nil Show; movies, tracks,
// and any program Tunarr didn't nest a show onto are left untouched.
func hydrateEpisodeShowFields(programs []Program) {
	for i := range programs {
		p := &programs[i]
		if p.Type != "episode" || p.Show == nil {
			continue
		}
		if p.ShowTitle == "" {
			p.ShowTitle = p.Show.Title
		}
		if p.Rating == "" {
			p.Rating = p.Show.Rating
		}
	}
}

// GetChannels retrieves all channels from the Tunarr instance.
// GET /api/channels
func (c *Client) GetChannels(ctx context.Context) ([]Channel, error) {
	const endpoint = "/api/channels"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	var channels []Channel
	if err := c.http.Get(ctx, endpoint, &channels); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get channels: %w", err)
	}

	for i := range channels {
		if err := validateChannel(&channels[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid channel in response: %w", err)
		}
	}

	return channels, nil
}

// GetMediaSources retrieves all connected media sources (Plex/Jellyfin/Emby).
// GET /api/media-sources
func (c *Client) GetMediaSources(ctx context.Context) ([]MediaSource, error) {
	const endpoint = "/api/media-sources"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	var sources []MediaSource
	if err := c.http.Get(ctx, endpoint, &sources); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get media sources: %w", err)
	}

	return sources, nil
}

// GetLibraries retrieves all libraries for a specific media source.
// GET /api/media-sources/{id}/libraries
func (c *Client) GetLibraries(ctx context.Context, mediaSourceID string) ([]Library, error) {
	const endpoint = "/api/media-sources/{id}/libraries"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if mediaSourceID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_media_source_id").Inc()
		return nil, errors.New("media source ID cannot be empty")
	}

	path := "/api/media-sources/" + mediaSourceID + "/libraries"
	var libraries []Library
	if err := c.http.Get(ctx, path, &libraries); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get libraries for media source %s: %w", mediaSourceID, err)
	}

	return libraries, nil
}

// filterValidPrograms validates each entry in programs, returning only the
// ones that pass and how many were dropped. This is the fix for a
// pre-existing bug: a single malformed or unrecognized entry anywhere in
// an otherwise large, valid page used to abort the ENTIRE response (see
// git history -- validateProgram's caller used to return an error the
// instant any one entry failed, which is exactly what happened once a
// growing library's search started interleaving "season"-type entries
// before Program.Type's validate oneof included "season": every
// accumulated page in that fetch got discarded, not just the one bad
// entry). SearchPrograms and GetFillerPrograms both call this instead of
// validating (and erroring) inline: one weird entry now costs exactly
// that one entry, never the whole batch. Each dropped entry still bumps
// the existing response_validation_error metrics counter; producing a
// human-readable summary is the caller's job -- see
// internal/service/schedule.go's fetchSingleLibrary/
// fetchAllProgramsViaSearch, which accumulate the dropped count across
// every page of a fetch and log one WARN per whole fetch, not one per
// page or per dropped entry.
func filterValidPrograms(programs []Program, endpoint, method string) ([]Program, int) {
	valid := make([]Program, 0, len(programs))
	dropped := 0
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			dropped++
			continue
		}
		valid = append(valid, programs[i])
	}
	return valid, dropped
}

// SearchPrograms searches for programs using the Tunarr search API.
// POST /api/programs/search
func (c *Client) SearchPrograms(ctx context.Context, req ProgramSearchRequest) (*ProgramSearchResponse, error) {
	const endpoint = "/api/programs/search"
	const method = http.MethodPost

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	var response ProgramSearchResponse
	if err := c.http.Post(ctx, endpoint, req, &response); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to search programs: %w", err)
	}

	hydrateEpisodeShowFields(response.Results)

	// See filterValidPrograms' doc comment: an invalid entry is dropped,
	// not fatal to this whole page.
	response.Results, response.DroppedCount = filterValidPrograms(response.Results, endpoint, method)

	return &response, nil
}

// GetFillerPrograms retrieves programs from a specific filler list. The
// second return is how many entries were dropped for failing validation
// (see filterValidPrograms' doc comment) -- a non-zero count is not an
// error, but callers should generally log it once (this is a single,
// non-paginated call, so "once per fetch" is trivially satisfied by
// logging at the call site; see internal/scheduler/engine.go's caller).
// GET /api/filler-lists/{id}/programs
func (c *Client) GetFillerPrograms(ctx context.Context, fillerListID string) ([]Program, int, error) {
	const endpoint = "/api/filler-lists/{id}/programs"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if fillerListID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_filler_list_id").Inc()
		return nil, 0, errors.New("filler list ID cannot be empty")
	}

	path := "/api/filler-lists/" + fillerListID + "/programs"
	var programs []Program
	if err := c.http.Get(ctx, path, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, 0, fmt.Errorf("failed to get filler programs for list %s: %w", fillerListID, err)
	}

	hydrateEpisodeShowFields(programs)

	valid, dropped := filterValidPrograms(programs, endpoint, method)

	return valid, dropped, nil
}

// GetSeason retrieves a single season by its UUID -- the only way to learn
// a live episode's season number (Season.SeasonNumber, the response's
// "index" field). GET /api/programming/seasons/{id}.
//
// service.Runner.resolveSeasonNumber (internal/service/schedule.go) is the
// only caller: a live Tunarr episode result carries no season number of
// its own at all, only a SeasonID foreign key (see Program.SeasonID's doc
// comment in models.go), so Runner resolves each distinct SeasonID it
// sees, once per cache window, through this endpoint. There is no bulk
// equivalent for internal SeasonID UUIDs: POST /api/programming/batch/lookup
// was checked against docs/tunarr/openapi.json and live -- it takes
// externalIds (source-specific strings like a Plex ratingKey, not Tunarr's
// internal UUIDs) and returns an unrelated, legacy "content" response
// shape, so it isn't usable here.
func (c *Client) GetSeason(ctx context.Context, seasonID string) (*Season, error) {
	const endpoint = "/api/programming/seasons/{id}"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if seasonID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_season_id").Inc()
		return nil, errors.New("season ID cannot be empty")
	}

	path := "/api/programming/seasons/" + seasonID
	var season Season
	if err := c.http.Get(ctx, path, &season); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get season %s: %w", seasonID, err)
	}

	return &season, nil
}

// channelStartTimePUTOmitKeys are the GET /api/channels/{id} response keys
// setChannelStartTime must strip before PUTting the object back: the PUT
// body schema (SaveableChannelSchema, types/src/schemas/channelSchema.ts
// at tag v1.3.13) is ChannelSchema.omit({fallback, programCount,
// transcoding, sessions}) -- GET-only/derived fields the write side
// doesn't accept. Zod silently strips unrecognized keys by default (no
// `.strict()` on either schema, checked this session), so leaving them in
// wouldn't actually break the PUT -- but dropping them keeps this call
// honest about what it's actually allowed to send, and avoids depending on
// that default.
var channelStartTimePUTOmitKeys = []string{"fallback", "programCount", "transcoding", "sessions"}

// setChannelStartTime anchors a channel's playback clock to anchor via a
// read-modify-write round trip against PUT /api/channels/{id}
// (SaveableChannelSchema) -- source-verified against tag v1.3.13
// (github.com/chrisbenincasa/tunarr): there is no dedicated
// "set-start-time" endpoint reachable over HTTP (ChannelDB.updateChannelStartTime
// and LineupRepository.setChannelPrograms's optional startTime parameter
// both exist server-side but are never called from any route in this
// version -- grepped the whole server/src tree, checked this session), so
// the only live-reachable way to change it is a full channel update. The
// route itself explicitly expects and reacts to a startTime change
// (channelsApi.ts's PUT handler compares old vs. new startTime to decide
// whether to regenerate the channel's lineup/guide), so this is not a
// workaround -- it's how Tunarr's own UI would make the same change.
//
// The response is decoded into a raw key/value map rather than a typed
// struct so this never has to model (and risk silently dropping) every
// field on a live Tunarr channel -- watermark, transcoding options,
// subtitle preferences, streamMode, and so on all round-trip untouched;
// only "startTime" is ever modified.
//
// See UpdateSchedule's doc comment for why this call has to happen at
// all: without it, a freshly-pushed lineup plays back from whatever
// position (now - channel.startTime) mod channel.duration happens to
// land on relative to the channel's PREVIOUS startTime, not from the
// wall-clock offsets baked into the lineup by
// service.buildAnchoredLineup.
func (c *Client) setChannelStartTime(ctx context.Context, channelID string, anchor time.Time) error {
	const endpoint = "/api/channels/{id}"
	path := "/api/channels/" + channelID

	getMethod := http.MethodGet
	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, getMethod).Inc()
	getTimer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, getMethod))
	var channel map[string]json.RawMessage
	getErr := c.http.Get(ctx, path, &channel)
	getTimer.ObserveDuration()
	if getErr != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, getMethod, "api_call_error").Inc()
		return fmt.Errorf("failed to fetch channel %s before anchoring start time: %w", channelID, getErr)
	}

	for _, key := range channelStartTimePUTOmitKeys {
		delete(channel, key)
	}
	// Tunarr's own write path (BasicChannelRepository.ts's
	// updateRequestToChannel, tag v1.3.13) truncates startTime down to the
	// whole minute server-side regardless of what's sent
	// (`dayjs(startTime).second(0).millisecond(0)`) -- truncate here too so
	// the value this call sends matches what buildAnchoredLineup's caller
	// used as offset zero, rather than silently drifting by up to 59.999s.
	startTimeMs, marshalErr := json.Marshal(anchor.Truncate(time.Minute).UnixMilli())
	if marshalErr != nil {
		return fmt.Errorf("failed to encode start time: %w", marshalErr)
	}
	channel["startTime"] = startTimeMs

	putMethod := http.MethodPut
	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, putMethod).Inc()
	putTimer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, putMethod))
	putErr := c.http.Put(ctx, path, channel, nil)
	putTimer.ObserveDuration()
	if putErr != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, putMethod, "api_call_error").Inc()
		return fmt.Errorf("failed to set start time for channel %s: %w", channelID, putErr)
	}

	return nil
}

// UpdateSchedule replaces a channel's entire Tunarr-visible timeline for
// the apply window anchor represents: it (1) anchors the channel's
// playback clock (setChannelStartTime, see its doc comment for why this
// is required and how it was verified) and then (2) pushes lineup as a
// full-replacement manual lineup: POST /api/channels/{id}/programming,
// body {"type": "manual", "lineup": [...], "append": false} -- see
// ManualLineupRequest's doc comment for how that contract was verified.
// There is no PUT route for the programming path against a live Tunarr
// 1.3.13 instance at all (confirmed live this session: 404 "Route PUT:...
// not found"); an earlier version of this method sent PUT there, which
// was the first bug this fixes.
//
// lineup must already be anchor-relative -- built by
// service.buildAnchoredLineup, the only caller, so that cumulative item
// durations from offset 0 equal each item's intended offset from anchor,
// with "flex" entries padding any gap and the whole lineup spanning at
// least the full apply window. UpdateSchedule itself does no timing
// math; it only transports what it's given. append is always false:
// this call is a full-replacement apply, not an incremental add -- see
// the "channel ownership" contract documented on this method and in
// docs/scheduling-concepts.md: every Tunarr-managed channel's entire
// timeline, including off-hours, is schedularr's alone to set on every
// apply, and anything else on that channel (content added through
// Tunarr's own UI, a leftover manual edit) is silently replaced.
func (c *Client) UpdateSchedule(ctx context.Context, channelID string, anchor time.Time, lineup []LineupItem) error {
	const endpoint = "/api/channels/{id}/programming"
	const method = http.MethodPost

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return errors.New("channel ID cannot be empty")
	}

	if err := c.setChannelStartTime(ctx, channelID, anchor); err != nil {
		return err
	}

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	path := "/api/channels/" + channelID + "/programming"
	req := ManualLineupRequest{Type: "manual", Lineup: lineup, Append: false}
	if err := c.http.Post(ctx, path, req, nil); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return fmt.Errorf("failed to update schedule for channel %s: %w", channelID, err)
	}

	return nil
}

// ChannelProgrammingResponse represents the response from GET /api/channels/{id}/programming.
type ChannelProgrammingResponse struct {
	Programs []ChannelProgram `json:"programs"`
}

// ChannelProgram represents a scheduled program in a channel's programming.
type ChannelProgram struct {
	// Program data
	Title         string  `json:"title"`
	Duration      float64 `json:"duration"` // milliseconds
	Type          string  `json:"type"`     // movie, episode, flex, redirect, custom
	Year          *int    `json:"year,omitempty"`
	Rating        string  `json:"rating,omitempty"`
	SeasonNumber  int     `json:"seasonNumber,omitempty"`
	EpisodeNumber int     `json:"episodeNumber,omitempty"`
	ShowTitle     string  `json:"showTitle,omitempty"`

	// Scheduling info
	StartTimeMs int64 `json:"startTime"` // Unix timestamp in milliseconds
}

// GetChannelProgramming retrieves the current programming for a channel.
// GET /api/channels/{id}/programming
func (c *Client) GetChannelProgramming(ctx context.Context, channelID string) ([]ChannelProgram, error) {
	const endpoint = "/api/channels/{id}/programming"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return nil, errors.New("channel ID cannot be empty")
	}

	path := "/api/channels/" + channelID + "/programming"
	var programs []ChannelProgram
	if err := c.http.Get(ctx, path, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get programming for channel %s: %w", channelID, err)
	}

	return programs, nil
}
