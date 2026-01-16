// Package jellyfin provides a client for interacting with the Jellyfin API.
//
// Authentication: Jellyfin supports multiple authentication methods:
//   - Authorization header with MediaBrowser token format (official)
//   - X-Emby-Token header (legacy/Emby compatibility)
//
// This client uses the official Authorization header format by default.
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
// Uses the official Authorization header format for authentication.
func NewClient(cfg Config) *Client {
	httpCfg := httpclient.DefaultConfig(
		strings.TrimRight(cfg.URL, "/"),
		cfg.APIKey,
		httpclient.AuthJellyfinAuthorization,
	)
	return &Client{
		http: httpclient.New(httpCfg),
	}
}

// NewClientLegacy creates a Jellyfin client using legacy X-Emby-Token authentication.
// Use this if your Jellyfin instance doesn't accept the Authorization header format.
func NewClientLegacy(cfg Config) *Client {
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
//
// Note: The /LiveTv/RefreshGuide endpoint is not documented in the official
// Jellyfin OpenAPI specification but is commonly used and supported.
// POST /LiveTv/RefreshGuide
func (c *Client) RefreshLiveTVGuide(ctx context.Context) error {
	if c.http.BaseURL() == "" {
		return errors.New("jellyfin base URL is empty")
	}

	if err := c.http.Post(ctx, "/LiveTv/RefreshGuide", nil, nil); err != nil {
		return fmt.Errorf("failed to refresh jellyfin live tv guide: %w", err)
	}

	return nil
}
