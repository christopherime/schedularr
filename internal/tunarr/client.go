// Package tunarr provides a client for interacting with the Tunarr API.
package tunarr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/geekxflood/schedularr/internal/metrics" // Added import
	"github.com/prometheus/client_golang/prometheus"    // Added import
)

// Client is a Tunarr API client.
type Client struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	maxRetries   int
	retryWaitMin time.Duration
	retryWaitMax time.Duration
}

// NewClient creates a new Tunarr API client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		baseURL:      cfg.URL,
		apiKey:       cfg.APIKey,
		maxRetries:   3,
		retryWaitMin: 1 * time.Second,
		retryWaitMax: 30 * time.Second,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Increased from 10s to account for retries
		},
	}
}

// newRequest creates a new HTTP request with necessary headers.
func (c *Client) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	url := c.baseURL + path

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		if err := json.NewEncoder(buf).Encode(body); err != nil {
			return nil, fmt.Errorf("failed to encode request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	return req, nil
}

// APIError represents an error response from the Tunarr API.
type APIError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
	Err        error
}

func (e *APIError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s %s failed with status %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s %s failed: %v", e.Method, e.URL, e.Err)
	}
	return fmt.Sprintf("%s %s failed with status %d", e.Method, e.URL, e.StatusCode)
}

func (e *APIError) Unwrap() error {
	return e.Err
}

// isRetryable determines if an HTTP status code should be retried.
func isRetryable(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout ||
		statusCode >= 500
}

// backoff calculates the exponential backoff duration for a retry attempt.
func (c *Client) backoff(attempt int) time.Duration {
	backoff := float64(c.retryWaitMin) * math.Pow(2, float64(attempt))
	if backoff > float64(c.retryWaitMax) {
		backoff = float64(c.retryWaitMax)
	}
	return time.Duration(backoff)
}

// do executes the HTTP request with retry logic and enhanced error handling.
func (c *Client) do(req *http.Request, v interface{}) error {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		bodyBytes := cloneRequestBody(req)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = &APIError{
				Method: req.Method,
				URL:    req.URL.String(),
				Err:    err,
			}
			if attempt < c.maxRetries {
				time.Sleep(c.backoff(attempt))
				resetRequestBody(req, bodyBytes)
				continue
			}
			return lastErr
		}

		if err := c.handleResponse(req, resp, v); err != nil {
			if shouldRetryStatus(err) && attempt < c.maxRetries {
				lastErr = err
				time.Sleep(c.backoff(attempt))
				resetRequestBody(req, bodyBytes)
				continue
			}
			return err
		}

		return nil
	}

	return lastErr
}

func cloneRequestBody(req *http.Request) []byte {
	if req.Body == nil {
		return nil
	}

	bodyBytes, _ := io.ReadAll(req.Body)
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	return bodyBytes
}

func resetRequestBody(req *http.Request, bodyBytes []byte) {
	if bodyBytes == nil {
		return
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
}

func (c *Client) handleResponse(req *http.Request, resp *http.Response, v interface{}) error {
	body, err := readResponseBody(resp)
	if err != nil {
		return &APIError{
			Method:     req.Method,
			URL:        req.URL.String(),
			StatusCode: resp.StatusCode,
			Err:        fmt.Errorf("failed to read response body: %w", err),
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{
			Method:     req.Method,
			URL:        req.URL.String(),
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return &APIError{
				Method:     req.Method,
				URL:        req.URL.String(),
				StatusCode: resp.StatusCode,
				Err:        fmt.Errorf("failed to decode response: %w", err),
			}
		}
	}

	return nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func shouldRetryStatus(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return isRetryable(apiErr.StatusCode)
	}
	return false
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

	req, err := c.newRequest(ctx, method, endpoint, nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create channels request: %w", err)
	}

	var channels []Channel
	if err := c.do(req, &channels); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get channels: %w", err)
	}

	// Validate response
	for i := range channels {
		if err := validateChannel(&channels[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid channel in response: %w", err)
		}
	}

	return channels, nil
}

// GetPrograms retrieves all available programs from the Tunarr instance.
// Note: This endpoint is a placeholder and may need to be updated based on actual Tunarr API.
func (c *Client) GetPrograms(ctx context.Context) ([]Program, error) {
	const endpoint = "/api/programs"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	req, err := c.newRequest(ctx, method, endpoint, nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create programs request: %w", err)
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get programs: %w", err)
	}

	// Validate response
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in response: %w", err)
		}
	}

	return programs, nil
}

// UpdateSchedule updates the programming schedule for a specific channel.
// Note: This endpoint is a placeholder and may need to be updated based on actual Tunarr API.
func (c *Client) UpdateSchedule(ctx context.Context, channelID string, schedule []Program) error {
	const endpoint = "/api/channels/schedule" // Generalized endpoint for metrics
	const method = http.MethodPost

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	if channelID == "" {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "invalid_channel_id").Inc()
		return errors.New("channel ID cannot be empty")
	}

	// Validate all programs in the schedule
	for i := range schedule {
		if err := validateProgram(&schedule[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "program_validation_error").Inc()
			return fmt.Errorf("invalid program in schedule at index %d: %w", i, err)
		}
	}

	req, err := c.newRequest(ctx, method, "/api/channels/"+channelID+"/schedule", schedule)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return fmt.Errorf("failed to create schedule update request: %w", err)
	}

	if err := c.do(req, nil); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return fmt.Errorf("failed to update schedule for channel %s: %w", channelID, err)
	}

	return nil
}

// GetLibraries retrieves all available media libraries from connected servers (Plex/Jellyfin/Emby).
func (c *Client) GetLibraries(ctx context.Context) ([]Library, error) {
	const endpoint = "/api/libraries"
	const method = http.MethodGet

	metrics.TunarrAPICallsTotal.WithLabelValues(endpoint, method).Inc()
	timer := prometheus.NewTimer(metrics.TunarrAPICallDurationSeconds.WithLabelValues(endpoint, method))
	defer timer.ObserveDuration()

	req, err := c.newRequest(ctx, method, endpoint, nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create libraries request: %w", err)
	}

	var libraries []Library
	if err := c.do(req, &libraries); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get libraries: %w", err)
	}

	return libraries, nil
}

// GetLibraryPrograms retrieves all programs from a specific library.
// This can be used to fetch content from Plex/Jellyfin/Emby libraries.
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

	req, err := c.newRequest(ctx, method, "/api/libraries/"+libraryID+"/programs", nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create library programs request: %w", err)
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get programs from library %s: %w", libraryID, err)
	}

	// Validate response
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

	req, err := c.newRequest(ctx, method, endpoint, nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create shows request: %w", err)
	}

	var shows []Show
	if err := c.do(req, &shows); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get shows: %w", err)
	}

	return shows, nil
}

// GetShowEpisodes retrieves episodes for a specific show.
// If season is 0, all episodes are returned. Otherwise, only episodes from that season.
func (c *Client) GetShowEpisodes(ctx context.Context, showID string, season int) ([]Program, error) {
	const endpoint = "/api/shows/{showID}/episodes" // Generalized endpoint for metrics
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

	req, err := c.newRequest(ctx, method, path, nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create show episodes request: %w", err)
	}

	var episodes []Program
	if err := c.do(req, &episodes); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get episodes for show %s: %w", showID, err)
	}

	// Validate response
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

	req, err := c.newRequest(ctx, method, "/api/programs/search?q="+query, nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to search programs with query '%s': %w", query, err)
	}

	// Validate response
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

	req, err := c.newRequest(ctx, method, endpoint, nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create filler lists request: %w", err)
	}

	var fillerLists []FillerList
	if err := c.do(req, &fillerLists); err != nil {
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

	req, err := c.newRequest(ctx, method, "/api/filler-lists/"+fillerListID+"/content", nil)
	if err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "request_creation_error").Inc()
		return nil, fmt.Errorf("failed to create filler content request: %w", err)
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "api_call_error").Inc()
		return nil, fmt.Errorf("failed to get filler content for list %s: %w", fillerListID, err)
	}

	// Validate response
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			metrics.TunarrAPIErrorsTotal.WithLabelValues(endpoint, method, "response_validation_error").Inc()
			return nil, fmt.Errorf("invalid program in filler content: %w", err)
		}
	}

	return programs, nil
}
