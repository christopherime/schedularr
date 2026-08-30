package httpclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	cfg := DefaultConfig("http://localhost:8080", "test-key", AuthXAPIKey)
	client := New(cfg)

	if client == nil {
		t.Fatal("expected client to be created")
	}
	if client.BaseURL() != "http://localhost:8080" {
		t.Errorf("expected base URL to be http://localhost:8080, got %s", client.BaseURL())
	}
}

func TestClient_Get(t *testing.T) {
	type testResponse struct {
		Message string `json:"message"`
		Count   int    `json:"count"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key header, got %s", r.Header.Get("X-API-Key"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(testResponse{Message: "hello", Count: 42})
	}))
	defer server.Close()

	cfg := DefaultConfig(server.URL, "test-key", AuthXAPIKey)
	client := New(cfg)

	var result testResponse
	err := client.Get(context.Background(), "/test", &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message != "hello" {
		t.Errorf("expected message 'hello', got %s", result.Message)
	}
	if result.Count != 42 {
		t.Errorf("expected count 42, got %d", result.Count)
	}
}

func TestClient_Post(t *testing.T) {
	type requestBody struct {
		Name string `json:"name"`
	}
	type responseBody struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.Header.Get("X-Emby-Token") != "emby-token" {
			t.Errorf("expected X-Emby-Token header, got %s", r.Header.Get("X-Emby-Token"))
		}

		var req requestBody
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(responseBody{ID: 1, Name: req.Name})
	}))
	defer server.Close()

	cfg := DefaultConfig(server.URL, "emby-token", AuthXEmbyToken)
	client := New(cfg)

	var result responseBody
	err := client.Post(context.Background(), "/items", requestBody{Name: "test"}, &result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ID != 1 {
		t.Errorf("expected ID 1, got %d", result.ID)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got %s", result.Name)
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedStatus int
	}{
		{"404 Not Found", http.StatusNotFound, http.StatusNotFound},
		{"401 Unauthorized", http.StatusUnauthorized, http.StatusUnauthorized},
		{"400 Bad Request", http.StatusBadRequest, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(`{"error": "test error"}`))
			}))
			defer server.Close()

			cfg := DefaultConfig(server.URL, "", AuthNone)
			cfg.MaxRetries = 0 // Disable retries for this test
			client := New(cfg)

			var result map[string]interface{}
			err := client.Get(context.Background(), "/test", &result)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			apiErr, ok := err.(*APIError)
			if !ok {
				t.Fatalf("expected APIError, got %T", err)
			}
			if apiErr.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, apiErr.StatusCode)
			}
		})
	}
}

func TestClient_NoAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "" {
			t.Error("expected no X-API-Key header")
		}
		if r.Header.Get("X-Emby-Token") != "" {
			t.Error("expected no X-Emby-Token header")
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("expected no Authorization header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultConfig(server.URL, "", AuthNone)
	client := New(cfg)

	err := client.Get(context.Background(), "/test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestClient_AuthBearer pins the RFC 6750 header shape AuthBearer sends.
// internal/metadata/tvdb depends on it: TheTVDB v4 mints a bearer token
// from a login call and rejects every other route without exactly this
// header.
func TestClient_AuthBearer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer minted-token" {
			t.Errorf("expected Authorization header %q, got %q", "Bearer minted-token", got)
		}
		if r.Header.Get("X-API-Key") != "" {
			t.Error("expected no X-API-Key header for a bearer client")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := New(DefaultConfig(server.URL, "minted-token", AuthBearer))

	if err := client.Get(context.Background(), "/test", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := DefaultConfig(server.URL, "", AuthNone)
	cfg.MaxRetries = 0
	client := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.Get(ctx, "/test", nil)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		apiErr   *APIError
		contains []string
	}{
		{
			name: "with body",
			apiErr: &APIError{
				Method:     "GET",
				URL:        "/api/test",
				StatusCode: 404,
				Body:       "not found",
			},
			contains: []string{"GET", "/api/test", "404", "not found"},
		},
		{
			name: "with error",
			apiErr: &APIError{
				Method: "POST",
				URL:    "/api/create",
				Err:    context.DeadlineExceeded,
			},
			contains: []string{"POST", "/api/create", "failed"},
		},
		{
			name: "status only",
			apiErr: &APIError{
				Method:     "DELETE",
				URL:        "/api/remove",
				StatusCode: 500,
			},
			contains: []string{"DELETE", "/api/remove", "500"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.apiErr.Error()
			for _, s := range tt.contains {
				if !strings.Contains(errMsg, s) {
					t.Errorf("expected error message to contain %q, got %q", s, errMsg)
				}
			}
		})
	}
}

func TestAPIError_Unwrap(t *testing.T) {
	underlying := context.DeadlineExceeded
	apiErr := &APIError{
		Method: "GET",
		URL:    "/test",
		Err:    underlying,
	}

	if apiErr.Unwrap() != underlying {
		t.Errorf("expected Unwrap to return %v, got %v", underlying, apiErr.Unwrap())
	}
}

func TestClient_Put(t *testing.T) {
	type updateRequest struct {
		Value int `json:"value"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT request, got %s", r.Method)
		}

		var req updateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}

		if req.Value != 100 {
			t.Errorf("expected value 100, got %d", req.Value)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := DefaultConfig(server.URL, "", AuthNone)
	client := New(cfg)

	err := client.Put(context.Background(), "/update", updateRequest{Value: 100}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestIsDecodeError_DistinguishesDecodeFromConnectivityAndStatus exercises
// IsDecodeError against the three real error shapes this client's
// resty-backed Get/Post/Put actually produce (verified this session, not
// assumed): a response body that fails to decode into the target struct
// (*APIError{StatusCode: 0, Err: *json.UnmarshalTypeError}), a
// connectivity failure (*APIError{StatusCode: 0, Err: *url.Error}), and a
// non-2xx HTTP status (*APIError{StatusCode: <code>, Err: nil}). Only the
// first must report true.
func TestIsDecodeError_DistinguishesDecodeFromConnectivityAndStatus(t *testing.T) {
	type target struct {
		Index int `json:"index"`
	}

	t.Run("decode failure (field type mismatch)", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"index": "not-a-number"}`))
		}))
		defer server.Close()

		cfg := DefaultConfig(server.URL, "", AuthNone)
		cfg.MaxRetries = 0
		client := New(cfg)

		var got target
		err := client.Get(context.Background(), "/x", &got)
		if err == nil {
			t.Fatal("expected a decode error, got nil")
		}
		if !IsDecodeError(err) {
			t.Errorf("expected IsDecodeError(err) to be true for a field type mismatch, got false (err: %v)", err)
		}
	})

	t.Run("connectivity failure", func(t *testing.T) {
		cfg := DefaultConfig("http://127.0.0.1:1", "", AuthNone)
		cfg.MaxRetries = 0
		client := New(cfg)

		var got target
		err := client.Get(context.Background(), "/x", &got)
		if err == nil {
			t.Fatal("expected a connectivity error, got nil")
		}
		if IsDecodeError(err) {
			t.Errorf("expected IsDecodeError(err) to be false for a connection failure, got true (err: %v)", err)
		}
	})

	t.Run("non-2xx status", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		}))
		defer server.Close()

		cfg := DefaultConfig(server.URL, "", AuthNone)
		cfg.MaxRetries = 0
		client := New(cfg)

		var got target
		err := client.Get(context.Background(), "/x", &got)
		if err == nil {
			t.Fatal("expected a status error, got nil")
		}
		if IsDecodeError(err) {
			t.Errorf("expected IsDecodeError(err) to be false for a non-2xx status, got true (err: %v)", err)
		}
	})

	t.Run("nil error", func(t *testing.T) {
		if IsDecodeError(nil) {
			t.Error("expected IsDecodeError(nil) to be false")
		}
	})

	t.Run("unrelated wrapped error", func(t *testing.T) {
		if IsDecodeError(errors.New("something else entirely")) {
			t.Error("expected IsDecodeError to be false for an unrelated error")
		}
	})
}
