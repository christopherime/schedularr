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
)

// Client is a Tunarr API client.
type Client struct {
	baseURL       string
	apiKey        string
	httpClient    *http.Client
	maxRetries    int
	retryWaitMin  time.Duration
	retryWaitMax  time.Duration
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
		// Clone request body for retries (if present)
		var bodyBytes []byte
		if req.Body != nil {
			bodyBytes, _ = io.ReadAll(req.Body)
			_ = req.Body.Close()
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = &APIError{
				Method: req.Method,
				URL:    req.URL.String(),
				Err:    err,
			}

			// Network errors are retryable
			if attempt < c.maxRetries {
				time.Sleep(c.backoff(attempt))
				// Restore request body for retry
				if bodyBytes != nil {
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
				continue
			}
			return lastErr
		}

		// Read response body for error reporting
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return &APIError{
				Method:     req.Method,
				URL:        req.URL.String(),
				StatusCode: resp.StatusCode,
				Err:        fmt.Errorf("failed to read response body: %w", err),
			}
		}

		// Check status code
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			apiErr := &APIError{
				Method:     req.Method,
				URL:        req.URL.String(),
				StatusCode: resp.StatusCode,
				Body:       string(body),
			}

			// Retry on retryable status codes
			if isRetryable(resp.StatusCode) && attempt < c.maxRetries {
				lastErr = apiErr
				time.Sleep(c.backoff(attempt))
				// Restore request body for retry
				if bodyBytes != nil {
					req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				}
				continue
			}

			return apiErr
		}

		// Decode response if destination is provided
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

	return lastErr
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
func (c *Client) GetChannels() ([]Channel, error) {
	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/channels", nil)
	if err != nil {
		return nil, err
	}

	var channels []Channel
	if err := c.do(req, &channels); err != nil {
		return nil, err
	}

	// Validate response
	for i := range channels {
		if err := validateChannel(&channels[i]); err != nil {
			return nil, fmt.Errorf("invalid channel in response: %w", err)
		}
	}

	return channels, nil
}

// GetPrograms retrieves all available programs from the Tunarr instance.
// Note: This endpoint is a placeholder and may need to be updated based on actual Tunarr API.
func (c *Client) GetPrograms() ([]Program, error) {
	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/programs", nil)
	if err != nil {
		return nil, err
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		return nil, err
	}

	// Validate response
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			return nil, fmt.Errorf("invalid program in response: %w", err)
		}
	}

	return programs, nil
}

// UpdateSchedule updates the programming schedule for a specific channel.
// Note: This endpoint is a placeholder and may need to be updated based on actual Tunarr API.
func (c *Client) UpdateSchedule(channelID string, schedule []Program) error {
	if channelID == "" {
		return errors.New("channel ID cannot be empty")
	}

	// Validate all programs in the schedule
	for i := range schedule {
		if err := validateProgram(&schedule[i]); err != nil {
			return fmt.Errorf("invalid program in schedule at index %d: %w", i, err)
		}
	}

	req, err := c.newRequest(context.Background(), http.MethodPost, "/api/channels/"+channelID+"/schedule", schedule)
	if err != nil {
		return err
	}

	return c.do(req, nil)
}

// GetLibraries retrieves all available media libraries from connected servers (Plex/Jellyfin/Emby).
func (c *Client) GetLibraries() ([]Library, error) {
	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/libraries", nil)
	if err != nil {
		return nil, err
	}

	var libraries []Library
	if err := c.do(req, &libraries); err != nil {
		return nil, err
	}

	return libraries, nil
}

// GetLibraryPrograms retrieves all programs from a specific library.
// This can be used to fetch content from Plex/Jellyfin/Emby libraries.
func (c *Client) GetLibraryPrograms(libraryID string) ([]Program, error) {
	if libraryID == "" {
		return nil, errors.New("library ID cannot be empty")
	}

	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/libraries/"+libraryID+"/programs", nil)
	if err != nil {
		return nil, err
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		return nil, err
	}

	// Validate response
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			return nil, fmt.Errorf("invalid program in library response: %w", err)
		}
	}

	return programs, nil
}

// GetShows retrieves all TV shows from the connected media servers.
func (c *Client) GetShows() ([]Show, error) {
	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/shows", nil)
	if err != nil {
		return nil, err
	}

	var shows []Show
	if err := c.do(req, &shows); err != nil {
		return nil, err
	}

	return shows, nil
}

// GetShowEpisodes retrieves episodes for a specific show.
// If season is 0, all episodes are returned. Otherwise, only episodes from that season.
func (c *Client) GetShowEpisodes(showID string, season int) ([]Program, error) {
	if showID == "" {
		return nil, errors.New("show ID cannot be empty")
	}

	path := "/api/shows/" + showID + "/episodes"
	if season > 0 {
		path = fmt.Sprintf("%s?season=%d", path, season)
	}

	req, err := c.newRequest(context.Background(), http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var episodes []Program
	if err := c.do(req, &episodes); err != nil {
		return nil, err
	}

	// Validate response
	for i := range episodes {
		if err := validateProgram(&episodes[i]); err != nil {
			return nil, fmt.Errorf("invalid episode in response: %w", err)
		}
	}

	return episodes, nil
}

// SearchPrograms searches for programs by title.
func (c *Client) SearchPrograms(query string) ([]Program, error) {
	if query == "" {
		return nil, errors.New("search query cannot be empty")
	}

	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/programs/search?q="+query, nil)
	if err != nil {
		return nil, err
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		return nil, err
	}

	// Validate response
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			return nil, fmt.Errorf("invalid program in search results: %w", err)
		}
	}

	return programs, nil
}

// GetFillerLists retrieves all available filler content lists.
func (c *Client) GetFillerLists() ([]FillerList, error) {
	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/filler-lists", nil)
	if err != nil {
		return nil, err
	}

	var fillerLists []FillerList
	if err := c.do(req, &fillerLists); err != nil {
		return nil, err
	}

	return fillerLists, nil
}

// GetFillerContent retrieves programs from a specific filler list.
func (c *Client) GetFillerContent(fillerListID string) ([]Program, error) {
	if fillerListID == "" {
		return nil, errors.New("filler list ID cannot be empty")
	}

	req, err := c.newRequest(context.Background(), http.MethodGet, "/api/filler-lists/"+fillerListID+"/content", nil)
	if err != nil {
		return nil, err
	}

	var programs []Program
	if err := c.do(req, &programs); err != nil {
		return nil, err
	}

	// Validate response
	for i := range programs {
		if err := validateProgram(&programs[i]); err != nil {
			return nil, fmt.Errorf("invalid program in filler content: %w", err)
		}
	}

	return programs, nil
}