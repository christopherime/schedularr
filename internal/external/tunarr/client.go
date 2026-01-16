// Package tunarr provides a client for interacting with the Tunarr API.
package tunarr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/geekxflood/schedularr/internal/httpclient"
	"github.com/geekxflood/schedularr/internal/metrics"
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

// GetChannelPrograms retrieves all programs for a specific channel.
// GET /api/channels/{id}/programs
func (c *Client) GetChannelPrograms(ctx context.Context, channelID string) ([]Program, error) {
	const endpoint = "/api/channels/{id}/programs"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return nil, errors.New("channel ID cannot be empty")
	}

	path := "/api/channels/" + channelID + "/programs"
	var programs []Program
	if err := c.http.Get(ctx, path, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get programs for channel %s: %w", channelID, err)
	}

	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in response: %w", err)
		}
	}

	return programs, nil
}

// GetChannelShows retrieves all TV shows for a specific channel.
// GET /api/channels/{id}/shows
func (c *Client) GetChannelShows(ctx context.Context, channelID string) ([]Show, error) {
	const endpoint = "/api/channels/{id}/shows"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return nil, errors.New("channel ID cannot be empty")
	}

	path := "/api/channels/" + channelID + "/shows"
	var shows []Show
	if err := c.http.Get(ctx, path, &shows); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get shows for channel %s: %w", channelID, err)
	}

	return shows, nil
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

	for i := range response.Results {
		if err := validateProgram(&response.Results[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in search results: %w", err)
		}
	}

	return &response, nil
}

// SearchProgramsByTitle is a convenience method to search programs by title.
func (c *Client) SearchProgramsByTitle(ctx context.Context, title string) ([]Program, error) {
	if title == "" {
		return nil, errors.New("search title cannot be empty")
	}

	req := ProgramSearchRequest{
		Query: &ProgramSearchQuery{
			Query: title,
		},
		Limit: 100,
	}

	resp, err := c.SearchPrograms(ctx, req)
	if err != nil {
		return nil, err
	}

	return resp.Results, nil
}

// GetFillerLists retrieves all available filler content lists.
// GET /api/filler-lists
func (c *Client) GetFillerLists(ctx context.Context) ([]FillerList, error) {
	const endpoint = "/api/filler-lists"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	var fillerLists []FillerList
	if err := c.http.Get(ctx, endpoint, &fillerLists); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get filler lists: %w", err)
	}

	return fillerLists, nil
}

// GetFillerPrograms retrieves programs from a specific filler list.
// GET /api/filler-lists/{id}/programs
func (c *Client) GetFillerPrograms(ctx context.Context, fillerListID string) ([]Program, error) {
	const endpoint = "/api/filler-lists/{id}/programs"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if fillerListID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_filler_list_id").Inc()
		return nil, errors.New("filler list ID cannot be empty")
	}

	path := "/api/filler-lists/" + fillerListID + "/programs"
	var programs []Program
	if err := c.http.Get(ctx, path, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get filler programs for list %s: %w", fillerListID, err)
	}

	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in filler programs: %w", err)
		}
	}

	return programs, nil
}

// GetChannelSchedule retrieves the current schedule for a channel.
// GET /api/channels/{id}/schedule
func (c *Client) GetChannelSchedule(ctx context.Context, channelID string) ([]Program, error) {
	const endpoint = "/api/channels/{id}/schedule"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return nil, errors.New("channel ID cannot be empty")
	}

	path := "/api/channels/" + channelID + "/schedule"
	var schedule []Program
	if err := c.http.Get(ctx, path, &schedule); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get schedule for channel %s: %w", channelID, err)
	}

	return schedule, nil
}

// UpdateScheduleTimeSlots updates the programming schedule for a channel using time slots.
// POST /api/channels/{channelId}/schedule-time-slots
func (c *Client) UpdateScheduleTimeSlots(ctx context.Context, channelID string, schedule *TimeBasedSchedule) error {
	const endpoint = "/api/channels/{channelId}/schedule-time-slots"
	const method = http.MethodPost

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return errors.New("channel ID cannot be empty")
	}

	if schedule == nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_schedule").Inc()
		return errors.New("schedule cannot be nil")
	}

	req := ScheduleRequest{Schedule: schedule}
	path := "/api/channels/" + channelID + "/schedule-time-slots"
	if err := c.http.Post(ctx, path, req, nil); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return fmt.Errorf("failed to update schedule for channel %s: %w", channelID, err)
	}

	return nil
}

// GetChannelLineup retrieves the lineup (schedule) for a channel.
// GET /api/channels/{id}/lineup
func (c *Client) GetChannelLineup(ctx context.Context, channelID string) ([]Program, error) {
	const endpoint = "/api/channels/{id}/lineup"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return nil, errors.New("channel ID cannot be empty")
	}

	path := "/api/channels/" + channelID + "/lineup"
	var lineup []Program
	if err := c.http.Get(ctx, path, &lineup); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get lineup for channel %s: %w", channelID, err)
	}

	return lineup, nil
}

// Deprecated method aliases for backward compatibility.
// These will be removed in a future version.

// GetPrograms is deprecated. Use GetChannelPrograms instead.
// This method now returns an error as the /api/programs endpoint does not exist.
func (c *Client) GetPrograms(ctx context.Context) ([]Program, error) {
	return nil, errors.New("GetPrograms is deprecated: /api/programs endpoint does not exist in Tunarr API; use GetChannelPrograms(channelID) or SearchPrograms() instead")
}

// GetLibraryPrograms is deprecated. Use GetChannelPrograms or SearchPrograms instead.
// The /api/libraries/{id}/programs endpoint does not exist in Tunarr API.
func (c *Client) GetLibraryPrograms(ctx context.Context, libraryID string) ([]Program, error) {
	return nil, errors.New("GetLibraryPrograms is deprecated: /api/libraries/{id}/programs endpoint does not exist in Tunarr API; use GetChannelPrograms(channelID) or SearchPrograms() instead")
}

// GetShows is deprecated. Use GetChannelShows instead.
// The /api/shows endpoint does not exist in Tunarr API.
func (c *Client) GetShows(ctx context.Context) ([]Show, error) {
	return nil, errors.New("GetShows is deprecated: /api/shows endpoint does not exist in Tunarr API; use GetChannelShows(channelID) instead")
}

// GetShowEpisodes is deprecated. Use SearchPrograms instead.
// The /api/shows/{id}/episodes endpoint does not exist in Tunarr API.
func (c *Client) GetShowEpisodes(ctx context.Context, showID string, season int) ([]Program, error) {
	return nil, errors.New("GetShowEpisodes is deprecated: /api/shows/{id}/episodes endpoint does not exist in Tunarr API; use SearchPrograms() instead")
}

// UpdateSchedule is deprecated. Use UpdateScheduleTimeSlots instead.
// The POST /api/channels/{id}/schedule endpoint does not accept a program array.
func (c *Client) UpdateSchedule(ctx context.Context, channelID string, schedule []Program) error {
	return errors.New("UpdateSchedule is deprecated: use UpdateScheduleTimeSlots() with a TimeBasedSchedule instead")
}

// GetFillerContent is deprecated. Use GetFillerPrograms instead.
// The endpoint path was incorrect (/content vs /programs).
func (c *Client) GetFillerContent(ctx context.Context, fillerListID string) ([]Program, error) {
	return c.GetFillerPrograms(ctx, fillerListID)
}
