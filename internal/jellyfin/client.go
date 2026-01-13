// Package jellyfin provides a client for interacting with the Jellyfin API.
package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a Jellyfin API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Jellyfin API client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	if c.baseURL == "" {
		return nil, errors.New("jellyfin base URL is empty")
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.apiKey != "" {
		req.Header.Set("X-Emby-Token", c.apiKey)
	}

	return req, nil
}

func (c *Client) do(req *http.Request) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("jellyfin request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read jellyfin response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("jellyfin request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// RefreshLiveTVGuide triggers a Live TV guide refresh.
func (c *Client) RefreshLiveTVGuide(ctx context.Context) error {
	req, err := c.newRequest(ctx, http.MethodPost, "/LiveTv/RefreshGuide")
	if err != nil {
		return fmt.Errorf("failed to create jellyfin refresh guide request: %w", err)
	}

	if err := c.do(req); err != nil {
		return fmt.Errorf("failed to refresh jellyfin live tv guide: %w", err)
	}

	return nil
}
