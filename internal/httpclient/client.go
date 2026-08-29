// Package httpclient provides a shared HTTP client with retry logic and metrics instrumentation.
package httpclient

import (
	"context"
	"encoding/json"
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
	// AuthXAPIKey uses X-API-Key header (Tunarr).
	AuthXAPIKey
	// AuthXEmbyToken uses X-Emby-Token header (Emby-compatible APIs).
	AuthXEmbyToken
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

// IsDecodeError reports whether err represents a response body that
// reached us and failed to decode into the expected shape (malformed
// JSON, or a field whose type didn't match what the target struct
// expects) -- as opposed to a connectivity failure (couldn't reach the
// server at all: DNS, dial, timeout) or a non-2xx HTTP status (server
// reached, but it reported its own error). Verified this session against
// this client's actual resty-backed behavior for all three cases: a
// decode failure surfaces as *APIError{StatusCode: 0, Err:
// *json.UnmarshalTypeError or *json.SyntaxError}; a connectivity failure
// surfaces as *APIError{StatusCode: 0, Err: *url.Error}; a non-2xx status
// surfaces as *APIError{StatusCode: <code>, Err: nil}. Distinguishing the
// first from the other two is what
// internal/api/media.go's writeMediaAPIError uses to stop saying "tunarr
// unreachable" for a failure where Tunarr was reached fine and the
// problem was a response shape schedularr didn't handle.
func IsDecodeError(err error) bool {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	return errors.As(err, &syntaxErr) || errors.As(err, &typeErr)
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
