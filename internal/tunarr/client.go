package tunarr

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"bytes"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: cfg.URL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) GetChannels() ([]Channel, error) {
	url := fmt.Sprintf("%s/api/channels", c.baseURL)
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

func (c *Client) GetPrograms() ([]Program, error) {
	// Placeholder endpoint - likely different in reality
	url := fmt.Sprintf("%s/api/programs", c.baseURL)
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

func (c *Client) UpdateSchedule(channelID string, schedule []Program) error {
	// Placeholder endpoint
	url := fmt.Sprintf("%s/api/channels/%s/schedule", c.baseURL, channelID)
	
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
