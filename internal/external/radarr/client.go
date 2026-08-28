// Package radarr provides a client for interacting with the Radarr API.
package radarr

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/christopherime/schedularr/internal/httpclient"
)

// Client is a Radarr API client.
type Client struct {
	http *httpclient.Client
}

// NewClient creates a new Radarr API client with the given configuration.
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

// GetMovies retrieves all movies from Radarr.
func (c *Client) GetMovies(ctx context.Context) ([]Movie, error) {
	if c.http.BaseURL() == "" {
		return nil, errors.New("radarr base URL is empty")
	}

	var movies []Movie
	if err := c.http.Get(ctx, "/api/v3/movie", &movies); err != nil {
		return nil, fmt.Errorf("failed to get radarr movies: %w", err)
	}

	return movies, nil
}
