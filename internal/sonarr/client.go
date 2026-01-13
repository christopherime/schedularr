// Package sonarr provides a client for interacting with the Sonarr API.
package sonarr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a Sonarr API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Sonarr API client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	if c.baseURL == "" {
		return nil, errors.New("sonarr base URL is empty")
	}
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
		req.Header.Set("X-Api-Key", c.apiKey)
	}

	return req, nil
}

func (c *Client) do(req *http.Request, v interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sonarr request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read sonarr response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sonarr request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return fmt.Errorf("failed to decode sonarr response: %w", err)
		}
	}

	return nil
}

// GetSeries retrieves all series from Sonarr.
func (c *Client) GetSeries(ctx context.Context) ([]Series, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v3/series", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create sonarr series request: %w", err)
	}

	var series []Series
	if err := c.do(req, &series); err != nil {
		return nil, fmt.Errorf("failed to get sonarr series: %w", err)
	}

	return series, nil
}

// GetEpisodes retrieves all episodes for a series from Sonarr.
func (c *Client) GetEpisodes(ctx context.Context, seriesID int) ([]Episode, error) {
	req, err := c.newRequest(ctx, http.MethodGet, fmt.Sprintf("/api/v3/episode?seriesId=%d", seriesID), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create sonarr episodes request: %w", err)
	}

	var episodes []Episode
	if err := c.do(req, &episodes); err != nil {
		return nil, fmt.Errorf("failed to get sonarr episodes for series %d: %w", seriesID, err)
	}

	return episodes, nil
}
