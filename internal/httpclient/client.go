// Package httpclient provides a shared HTTP client with retry logic and metrics instrumentation.
package httpclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client wraps resty.Client with common configuration for API clients.
type Client struct {
	resty    *resty.Client
	baseURL  string
	authType AuthType
	apiKey   string
}

// AuthType represents the type of authentication to use.
type AuthType int

const (
	// AuthNone indicates no authentication.
	AuthNone AuthType = iota
	// AuthXAPIKey uses X-API-Key header (Tunarr, Radarr, Sonarr).
	AuthXAPIKey
	// AuthXEmbyToken uses X-Emby-Token header (Jellyfin legacy/Emby compatibility).
	AuthXEmbyToken
	// AuthJellyfinAuthorization uses Authorization header with MediaBrowser token (Jellyfin official).
	AuthJellyfinAuthorization
)

// Config holds the configuration for creating a new HTTP client.
type Config struct {
	BaseURL      string
	APIKey       string
	AuthType     AuthType
	Timeout      time.Duration
	MaxRetries   int
	RetryWaitMin time.Duration
	RetryWaitMax time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(baseURL, apiKey string, authType AuthType) Config {
	return Config{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		AuthType:     authType,
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		RetryWaitMin: 1 * time.Second,
		RetryWaitMax: 30 * time.Second,
	}
}

// New creates a new HTTP client with the given configuration.
func New(cfg Config) *Client {
	c := &Client{
		baseURL:  cfg.BaseURL,
		authType: cfg.AuthType,
		apiKey:   cfg.APIKey,
	}

	r := resty.New().
		SetTimeout(cfg.Timeout).
		SetRetryCount(cfg.MaxRetries).
		SetRetryWaitTime(cfg.RetryWaitMin).
		SetRetryMaxWaitTime(cfg.RetryWaitMax).
		SetRetryAfter(nil). // Use default exponential backoff
		AddRetryCondition(func(r *resty.Response, err error) bool {
			if err != nil {
				return true
			}
			return isRetryableStatus(r.StatusCode())
		}).
		OnBeforeRequest(c.addAuthHeader)

	c.resty = r
	return c
}

// addAuthHeader is a middleware that adds authentication headers to requests.
func (c *Client) addAuthHeader(_ *resty.Client, req *resty.Request) error {
	if c.apiKey == "" {
		return nil
	}

	switch c.authType {
	case AuthXAPIKey:
		req.SetHeader("X-API-Key", c.apiKey)
	case AuthXEmbyToken:
		req.SetHeader("X-Emby-Token", c.apiKey)
	case AuthJellyfinAuthorization:
		// Jellyfin official API uses Authorization header with MediaBrowser token format
		req.SetHeader("Authorization", "MediaBrowser Token=\""+c.apiKey+"\"")
	}
	return nil
}

// isRetryableStatus determines if an HTTP status code should trigger a retry.
func isRetryableStatus(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusServiceUnavailable ||
		statusCode == http.StatusGatewayTimeout ||
		statusCode >= 500
}

// APIError represents an error response from an API.
type APIError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
	Err        error
}

func (e *APIError) Error() string {
	if e.Body != "" {
		return fmt.Sprintf("%s %s failed with status %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s %s failed: %v", e.Method, e.URL, e.Err)
	}
	return fmt.Sprintf("%s %s failed with status %d", e.Method, e.URL, e.StatusCode)
}

// Unwrap returns the underlying error.
func (e *APIError) Unwrap() error {
	return e.Err
}

// newRequest creates a new resty request with context.
// Authentication headers are added automatically via middleware.
func (c *Client) newRequest(ctx context.Context) *resty.Request {
	return c.resty.R().SetContext(ctx)
}

// Get performs a GET request and unmarshals the response into result.
func (c *Client) Get(ctx context.Context, path string, result interface{}) error {
	url := c.baseURL + path
	resp, err := c.newRequest(ctx).
		SetResult(result).
		Get(url)

	return c.handleResponse(resp, err, http.MethodGet, url)
}

// Post performs a POST request with the given body and unmarshals the response into result.
func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path
	req := c.newRequest(ctx)

	if body != nil {
		req.SetBody(body)
	}
	if result != nil {
		req.SetResult(result)
	}

	resp, err := req.Post(url)
	return c.handleResponse(resp, err, http.MethodPost, url)
}

// Put performs a PUT request with the given body and unmarshals the response into result.
func (c *Client) Put(ctx context.Context, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path
	req := c.newRequest(ctx)

	if body != nil {
		req.SetBody(body)
	}
	if result != nil {
		req.SetResult(result)
	}

	resp, err := req.Put(url)
	return c.handleResponse(resp, err, http.MethodPut, url)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) error {
	url := c.baseURL + path
	resp, err := c.newRequest(ctx).Delete(url)
	return c.handleResponse(resp, err, http.MethodDelete, url)
}

// handleResponse processes the response and returns an appropriate error if needed.
func (c *Client) handleResponse(resp *resty.Response, err error, method, url string) error {
	if err != nil {
		return &APIError{
			Method: method,
			URL:    url,
			Err:    err,
		}
	}

	if resp.IsError() {
		return &APIError{
			Method:     method,
			URL:        url,
			StatusCode: resp.StatusCode(),
			Body:       resp.String(),
		}
	}

	return nil
}

// BaseURL returns the base URL of the client.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// SetBaseURL updates the base URL of the client.
func (c *Client) SetBaseURL(url string) {
	c.baseURL = url
}

// ValidateRequired validates that required string fields are non-empty.
func ValidateRequired(name, value string) error {
	if value == "" {
		return errors.New(name + " cannot be empty")
	}
	return nil
}
