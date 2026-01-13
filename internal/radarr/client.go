// Package radarr provides a client for interacting with the Radarr API.
package radarr

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

// Client is a Radarr API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Radarr API client with the given configuration.
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
		return nil, errors.New("radarr base URL is empty")
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
		return fmt.Errorf("radarr request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read radarr response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("radarr request failed with status %d: %s", resp.StatusCode, string(body))
	}

	if v != nil {
		if err := json.Unmarshal(body, v); err != nil {
			return fmt.Errorf("failed to decode radarr response: %w", err)
		}
	}

	return nil
}

// GetMovies retrieves all movies from Radarr.
func (c *Client) GetMovies(ctx context.Context) ([]Movie, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/api/v3/movie", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create radarr movies request: %w", err)
	}

	var movies []Movie
	if err := c.do(req, &movies); err != nil {
		return nil, fmt.Errorf("failed to get radarr movies: %w", err)
	}

	return movies, nil
}
