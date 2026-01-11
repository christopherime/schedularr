// Package tunarr provides a client for interacting with the Tunarr API.
package tunarr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a Tunarr API client.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new Tunarr API client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: cfg.URL,
		apiKey:  cfg.APIKey,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
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

// do executes the HTTP request and handles common error cases.
func (c *Client) do(req *http.Request, v interface{}) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("api returned non-ok status: %d", resp.StatusCode)
	}

	if v != nil {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
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

	return programs, nil
}

// UpdateSchedule updates the programming schedule for a specific channel.
// Note: This endpoint is a placeholder and may need to be updated based on actual Tunarr API.
func (c *Client) UpdateSchedule(channelID string, schedule []Program) error {
	req, err := c.newRequest(context.Background(), http.MethodPost, "/api/channels/"+channelID+"/schedule", schedule)
	if err != nil {
		return err
	}

	return c.do(req, nil)
}