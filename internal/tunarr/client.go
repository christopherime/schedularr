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

// validateChannel validates a Channel struct for required fields.
func validateChannel(ch *Channel) error {
	if ch.ID == "" {
		return errors.New("channel missing required field: id")
	}
	if ch.Name == "" {
		return fmt.Errorf("channel %s missing required field: name", ch.ID)
	}
	return nil
}

// validateProgram validates a Program struct for required fields.
func validateProgram(p *Program) error {
	if p.ID == "" {
		return errors.New("program missing required field: id")
	}
	if p.Title == "" {
		return fmt.Errorf("program %s missing required field: title", p.ID)
	}
	if p.Duration <= 0 {
		return fmt.Errorf("program %s has invalid duration: %d", p.ID, p.Duration)
	}
	if p.Type != "movie" && p.Type != "episode" && p.Type != "track" && p.Type != "" {
		return fmt.Errorf("program %s has invalid type: %s", p.ID, p.Type)
	}
	return nil
}

// GetChannels retrieves all channels from the Tunarr instance.
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

// GetPrograms retrieves all available programs from the Tunarr instance.
func (c *Client) GetPrograms(ctx context.Context) ([]Program, error) {
	const endpoint = "/api/programs"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	var programs []Program
	if err := c.http.Get(ctx, endpoint, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get programs: %w", err)
	}

	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in response: %w", err)
		}
	}

	return programs, nil
}

// UpdateSchedule updates the programming schedule for a specific channel.
func (c *Client) UpdateSchedule(ctx context.Context, channelID string, schedule []Program) error {
	const endpoint = "/api/channels/schedule"
	const method = http.MethodPost

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return errors.New("channel ID cannot be empty")
	}

	for i := range schedule {
		if err := validateProgram(&schedule[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "program_validation_error").Inc()
			return fmt.Errorf("invalid program in schedule at index %d: %w", i, err)
		}
	}

	path := "/api/channels/" + channelID + "/schedule"
	if err := c.http.Post(ctx, path, schedule, nil); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return fmt.Errorf("failed to update schedule for channel %s: %w", channelID, err)
	}

	return nil
}

// GetLibraries retrieves all available media libraries from connected servers.
func (c *Client) GetLibraries(ctx context.Context) ([]Library, error) {
	const endpoint = "/api/libraries"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	var libraries []Library
	if err := c.http.Get(ctx, endpoint, &libraries); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get libraries: %w", err)
	}

	return libraries, nil
}

// GetLibraryPrograms retrieves all programs from a specific library.
func (c *Client) GetLibraryPrograms(ctx context.Context, libraryID string) ([]Program, error) {
	const endpoint = "/api/libraries/{libraryID}/programs"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if libraryID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_library_id").Inc()
		return nil, errors.New("library ID cannot be empty")
	}

	path := "/api/libraries/" + libraryID + "/programs"
	var programs []Program
	if err := c.http.Get(ctx, path, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get programs from library %s: %w", libraryID, err)
	}

	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in library response: %w", err)
		}
	}

	return programs, nil
}

// GetShows retrieves all TV shows from the connected media servers.
func (c *Client) GetShows(ctx context.Context) ([]Show, error) {
	const endpoint = "/api/shows"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	var shows []Show
	if err := c.http.Get(ctx, endpoint, &shows); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get shows: %w", err)
	}

	return shows, nil
}

// GetShowEpisodes retrieves episodes for a specific show.
// If season is 0, all episodes are returned. Otherwise, only episodes from that season.
func (c *Client) GetShowEpisodes(ctx context.Context, showID string, season int) ([]Program, error) {
	const endpoint = "/api/shows/{showID}/episodes"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if showID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_show_id").Inc()
		return nil, errors.New("show ID cannot be empty")
	}

	path := "/api/shows/" + showID + "/episodes"
	if season > 0 {
		path = fmt.Sprintf("%s?season=%d", path, season)
	}

	var episodes []Program
	if err := c.http.Get(ctx, path, &episodes); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get episodes for show %s: %w", showID, err)
	}

	for i := range episodes {
		if err := validateProgram(&episodes[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid episode in response: %w", err)
		}
	}

	return episodes, nil
}

// SearchPrograms searches for programs by title.
func (c *Client) SearchPrograms(ctx context.Context, query string) ([]Program, error) {
	const endpoint = "/api/programs/search"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if query == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "empty_query").Inc()
		return nil, errors.New("search query cannot be empty")
	}

	path := "/api/programs/search?q=" + query
	var programs []Program
	if err := c.http.Get(ctx, path, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to search programs with query '%s': %w", query, err)
	}

	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in search results: %w", err)
		}
	}

	return programs, nil
}

// GetFillerLists retrieves all available filler content lists.
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

// GetFillerContent retrieves programs from a specific filler list.
func (c *Client) GetFillerContent(ctx context.Context, fillerListID string) ([]Program, error) {
	const endpoint = "/api/filler-lists/{fillerListID}/content"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if fillerListID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_filler_list_id").Inc()
		return nil, errors.New("filler list ID cannot be empty")
	}

	path := "/api/filler-lists/" + fillerListID + "/content"
	var programs []Program
	if err := c.http.Get(ctx, path, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get filler content for list %s: %w", fillerListID, err)
	}

	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in filler content: %w", err)
		}
	}

	return programs, nil
}
