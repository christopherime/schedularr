// Package tunarr provides a client for interacting with the Tunarr API.
package tunarr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is a Tunarr API client.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Tunarr API client with the given configuration.
func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: cfg.URL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetChannels retrieves all channels from the Tunarr instance.
func (c *Client) GetChannels() ([]Channel, error) {
	url := c.baseURL + "/api/channels"
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch channels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned non-ok status: %d", resp.StatusCode)
	}

	var channels []Channel
	if err := json.NewDecoder(resp.Body).Decode(&channels); err != nil {
		return nil, fmt.Errorf("failed to decode channels response: %w", err)
	}

	return channels, nil
}

// GetPrograms retrieves all available programs from the Tunarr instance.
// Note: This endpoint is a placeholder and may need to be updated based on actual Tunarr API.
func (c *Client) GetPrograms() ([]Program, error) {
	url := c.baseURL + "/api/programs"
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch programs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api returned non-ok status: %d", resp.StatusCode)
	}

	var programs []Program
	if err := json.NewDecoder(resp.Body).Decode(&programs); err != nil {
		return nil, fmt.Errorf("failed to decode programs response: %w", err)
	}

	return programs, nil
}

// UpdateSchedule updates the programming schedule for a specific channel.
// Note: This endpoint is a placeholder and may need to be updated based on actual Tunarr API.
func (c *Client) UpdateSchedule(channelID string, schedule []Program) error {
	url := c.baseURL + "/api/channels/" + channelID + "/schedule"

	// Create payload - likely list of content IDs or program objects
	// Tunarr API specifics needed here. Assuming sending full objects or IDs.
	body, err := json.Marshal(schedule)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to update schedule: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("api returned non-ok status: %d", resp.StatusCode)
	}

	return nil
}
