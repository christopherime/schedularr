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

// UpdateSchedule applies a schedule of programs to a channel.
// PUT /api/channels/{id}/programming
func (c *Client) UpdateSchedule(ctx context.Context, channelID string, programs []Program) error {
	const endpoint = "/api/channels/{id}/programming"
	const method = http.MethodPut

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return errors.New("channel ID cannot be empty")
	}

	// Validate all programs before sending
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_validation_error").Inc()
			return fmt.Errorf("invalid program at index %d: %w", i, err)
		}
	}

	path := "/api/channels/" + channelID + "/programming"
	if err := c.http.Put(ctx, path, programs, nil); err != nil {
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
