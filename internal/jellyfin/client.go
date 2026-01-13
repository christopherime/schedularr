// Package jellyfin provides a client for interacting with the Jellyfin API.
package jellyfin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/geekxflood/schedularr/internal/httpclient"
)

// Client is a Jellyfin API client.
type Client struct {
	http *httpclient.Client
}

// NewClient creates a new Jellyfin API client with the given configuration.
func NewClient(cfg Config) *Client {
	httpCfg := httpclient.DefaultConfig(
		strings.TrimRight(cfg.URL, "/"),
		cfg.APIKey,
		httpclient.AuthXEmbyToken,
	)
	return &Client{
		http: httpclient.New(httpCfg),
	}
}

// RefreshLiveTVGuide triggers a Live TV guide refresh.
func (c *Client) RefreshLiveTVGuide(ctx context.Context) error {
	if c.http.BaseURL() == "" {
		return errors.New("jellyfin base URL is empty")
	}

	if err := c.http.Post(ctx, "/LiveTv/RefreshGuide", nil, nil); err != nil {
		return fmt.Errorf("failed to refresh jellyfin live tv guide: %w", err)
	}

	return nil
}
