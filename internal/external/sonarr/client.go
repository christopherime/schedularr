// Package sonarr provides a client for interacting with the Sonarr API.
package sonarr

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/christopherime/schedularr/internal/httpclient"
)

// Client is a Sonarr API client.
type Client struct {
	http *httpclient.Client
}

// NewClient creates a new Sonarr API client with the given configuration.
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

// GetSeries retrieves all series from Sonarr.
func (c *Client) GetSeries(ctx context.Context) ([]Series, error) {
	if c.http.BaseURL() == "" {
		return nil, errors.New("sonarr base URL is empty")
	}

	var series []Series
	if err := c.http.Get(ctx, "/api/v3/series", &series); err != nil {
		return nil, fmt.Errorf("failed to get sonarr series: %w", err)
	}

	return series, nil
}

// GetEpisodes retrieves all episodes for a series from Sonarr.
func (c *Client) GetEpisodes(ctx context.Context, seriesID int) ([]Episode, error) {
	if c.http.BaseURL() == "" {
		return nil, errors.New("sonarr base URL is empty")
	}

	path := fmt.Sprintf("/api/v3/episode?seriesId=%d", seriesID)
	var episodes []Episode
	if err := c.http.Get(ctx, path, &episodes); err != nil {
		return nil, fmt.Errorf("failed to get sonarr episodes for series %d: %w", seriesID, err)
	}

	return episodes, nil
}
