package tunarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/geekxflood/schedularr/internal/httpclient"
)

func TestClient_GetChannels(t *testing.T) {
	// mock response data
	mockChannels := []Channel{
		{
			ID:     "channel-1",
			Number: 1,
			Name:   "Test Channel 1",
			Icon:   &ChannelIcon{Path: "http://example.com/icon1.png"},
		},
		{
			ID:     "channel-2",
			Number: 2,
			Name:   "Test Channel 2",
			Icon:   &ChannelIcon{Path: "http://example.com/icon2.png"},
		},
	}

	// create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// check request method and path
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/channels" {
			t.Errorf("expected /api/channels path, got %s", r.URL.Path)
		}

		// check auth header
		if r.Header.Get("X-API-Key") != "test-api-key" {
			t.Errorf("expected X-API-Key header to be test-api-key, got %s", r.Header.Get("X-API-Key"))
		}

		// return mock data
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(mockChannels); err != nil {
			t.Fatalf("failed to encode mock response: %v", err)
		}
	}))
	defer server.Close()

	// create client with mock server url
	cfg := Config{
		URL:    server.URL,
		APIKey: "test-api-key",
	}
	client := NewClient(cfg)

	// test GetChannels
	channels, err := client.GetChannels(context.Background())
	if err != nil {
		t.Fatalf("GetChannels returned error: %v", err)
	}

	// verify response
	if len(channels) != len(mockChannels) {
		t.Errorf("expected %d channels, got %d", len(mockChannels), len(channels))
	}

	for i, ch := range channels {
		if !reflect.DeepEqual(ch, mockChannels[i]) {
			t.Errorf("channel %d mismatch: expected %+v, got %+v", i, mockChannels[i], ch)
		}
	}
}

func TestClient_GetPrograms_Deprecated(t *testing.T) {
	// GetPrograms is deprecated - it should return an error
	client := NewClient(Config{URL: "http://localhost"})
	_, err := client.GetPrograms(context.Background())
	if err == nil {
		t.Error("Expected deprecation error, got nil")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("Expected error to mention deprecation, got: %v", err)
	}
}

func TestClient_UpdateSchedule_Deprecated(t *testing.T) {
	// UpdateSchedule is deprecated - it should return an error
	client := NewClient(Config{URL: "http://localhost"})
	schedule := []Program{
		{ID: "prog-1", Title: "Show A", Duration: 1800000, Type: "episode"},
	}
	err := client.UpdateSchedule(context.Background(), "channel-1", schedule)
	if err == nil {
		t.Error("Expected deprecation error, got nil")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("Expected error to mention deprecation, got: %v", err)
	}
}

func TestClient_GetChannels_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := Config{
		URL: server.URL,
	}
	client := NewClient(cfg)

	_, err := client.GetChannels(context.Background())
	if err == nil {
		t.Error("expected error from non-200 status code, got nil")
	}
}

func TestClient_GetLibraries(t *testing.T) {
	mockLibraries := []Library{
		{ID: "lib-1", Name: "Movies", MediaType: "movies"},
		{ID: "lib-2", Name: "TV Shows", MediaType: "shows"},
	}
	mediaSourceID := "source-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		expectedPath := "/api/media-sources/" + mediaSourceID + "/libraries"
		if r.URL.Path != expectedPath {
			t.Errorf("expected %s path, got %s", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockLibraries)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	libraries, err := client.GetLibraries(context.Background(), mediaSourceID)
	if err != nil {
		t.Fatalf("GetLibraries returned error: %v", err)
	}

	if len(libraries) != len(mockLibraries) {
		t.Errorf("expected %d libraries, got %d", len(mockLibraries), len(libraries))
	}
}

func TestClient_GetLibraryPrograms_Deprecated(t *testing.T) {
	client := NewClient(Config{URL: "http://localhost"})
	_, err := client.GetLibraryPrograms(context.Background(), "lib-1")
	if err == nil {
		t.Error("Expected deprecation error, got nil")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("Expected error to mention deprecation, got: %v", err)
	}
}

func TestClient_GetShows_Deprecated(t *testing.T) {
	client := NewClient(Config{URL: "http://localhost"})
	_, err := client.GetShows(context.Background())
	if err == nil {
		t.Error("Expected deprecation error, got nil")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("Expected error to mention deprecation, got: %v", err)
	}
}

func TestClient_GetShowEpisodes_Deprecated(t *testing.T) {
	client := NewClient(Config{URL: "http://localhost"})
	_, err := client.GetShowEpisodes(context.Background(), "show-1", 1)
	if err == nil {
		t.Error("Expected deprecation error, got nil")
	}
	if !strings.Contains(err.Error(), "deprecated") {
		t.Errorf("Expected error to mention deprecation, got: %v", err)
	}
}

func TestClient_SearchPrograms(t *testing.T) {
	query := "Star"
	mockResponse := ProgramSearchResponse{
		Results: []Program{
			{ID: "prog-1", Title: "Star Wars", Duration: 7200000, Type: "movie"},
			{ID: "prog-2", Title: "Star Trek", Duration: 6000000, Type: "movie"},
		},
		Total: 2,
		Page:  1,
		Limit: 100,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/programs/search" {
			t.Errorf("expected /api/programs/search path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	response, err := client.SearchPrograms(context.Background(), ProgramSearchRequest{Query: &ProgramSearchQuery{Query: query}})
	if err != nil {
		t.Fatalf("SearchPrograms returned error: %v", err)
	}

	if len(response.Results) != len(mockResponse.Results) {
		t.Errorf("expected %d programs, got %d", len(mockResponse.Results), len(response.Results))
	}
}

func TestClient_GetFillerLists(t *testing.T) {
	mockFillers := []FillerList{
		{ID: "filler-1", Name: "Commercials", ContentCount: 50},
		{ID: "filler-2", Name: "Bumpers", ContentCount: 30},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/filler-lists" {
			t.Errorf("expected /api/filler-lists path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockFillers)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	fillers, err := client.GetFillerLists(context.Background())
	if err != nil {
		t.Fatalf("GetFillerLists returned error: %v", err)
	}

	if len(fillers) != len(mockFillers) {
		t.Errorf("expected %d filler lists, got %d", len(mockFillers), len(fillers))
	}
}

func TestClient_GetFillerContent(t *testing.T) {
	fillerID := "filler-1"
	mockContent := []Program{
		{ID: "prog-1", Title: "Commercial A", Duration: 30000, Type: "track"},
		{ID: "prog-2", Title: "Commercial B", Duration: 30000, Type: "track"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		expectedPath := "/api/filler-lists/" + fillerID + "/programs"
		if r.URL.Path != expectedPath {
			t.Errorf("expected %s path, got %s", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockContent)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	content, err := client.GetFillerContent(context.Background(), fillerID)
	if err != nil {
		t.Fatalf("GetFillerContent returned error: %v", err)
	}

	if len(content) != len(mockContent) {
		t.Errorf("expected %d programs, got %d", len(mockContent), len(content))
	}
}

func TestClient_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		responseBody   string
		expectedErrMsg string
	}{
		{
			name:           "404 Not Found",
			statusCode:     http.StatusNotFound,
			responseBody:   `{"error": "not found"}`,
			expectedErrMsg: "404",
		},
		{
			name:           "500 Internal Server Error",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error": "internal error"}`,
			expectedErrMsg: "500",
		},
		{
			name:           "401 Unauthorized",
			statusCode:     http.StatusUnauthorized,
			responseBody:   `{"error": "unauthorized"}`,
			expectedErrMsg: "401",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := NewClient(Config{URL: server.URL})
			_, err := client.GetChannels(context.Background())
			if err == nil {
				t.Error("Expected error, got nil")
			}
			if err != nil && !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error to contain %q, got %q", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

func TestClient_ValidationErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return invalid channel data
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]Channel{
			{ID: "", Name: "Invalid Channel"}, // Missing required ID
		})
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetChannels(context.Background())
	if err == nil {
		t.Error("Expected validation error for invalid channel, got nil")
	}
}

func TestClient_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("invalid json"))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetChannels(context.Background())
	if err == nil {
		t.Error("Expected JSON decode error, got nil")
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.GetChannels(ctx)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

// Note: APIError tests have been moved to internal/httpclient/client_test.go
// as APIError is now part of the shared httpclient package.

func TestClient_UpdateSchedule_ValidationError(t *testing.T) {
	channelID := "channel-1"
	// Schedule with invalid program (missing required fields)
	schedule := []Program{
		{ID: "", Title: "Invalid Program", Duration: 0}, // Missing ID and invalid duration
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Should not reach server due to validation error")
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	err := client.UpdateSchedule(context.Background(), channelID, schedule)
	if err == nil {
		t.Error("Expected validation error for invalid program, got nil")
	}
}

func TestClient_UpdateSchedule_EmptyChannelID(t *testing.T) {
	schedule := []Program{
		{ID: "prog-1", Title: "Show A", Duration: 1800000, Type: "episode"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Should not reach server due to empty channel ID")
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	err := client.UpdateSchedule(context.Background(), "", schedule)
	if err == nil {
		t.Error("Expected error for empty channel ID, got nil")
	}
}

func TestClient_GetLibraries_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetLibraries(context.Background(), "source-1")
	if err == nil {
		t.Error("Expected error for 500 response, got nil")
	}
}

// Note: GetLibraryPrograms, GetShows, and GetShowEpisodes are deprecated.
// The following tests verify they return deprecation errors as expected.
// See TestClient_GetLibraryPrograms_Deprecated, TestClient_GetShows_Deprecated,
// and TestClient_GetShowEpisodes_Deprecated for the main deprecation tests.

func TestClient_SearchPrograms_EmptyQuery(t *testing.T) {
	// Note: The current implementation doesn't validate empty queries client-side.
	// The search will proceed and return results based on filters.
	// This test validates that the API call completes without error.
	mockResponse := ProgramSearchResponse{
		Results: []Program{},
		Total:   0,
		Page:    1,
		Limit:   100,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.SearchPrograms(context.Background(), ProgramSearchRequest{Query: &ProgramSearchQuery{Query: ""}})
	if err != nil {
		t.Errorf("SearchPrograms with empty query should not fail: %v", err)
	}
}

func TestClient_SearchPrograms_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.SearchPrograms(context.Background(), ProgramSearchRequest{Query: &ProgramSearchQuery{Query: "test"}})
	if err == nil {
		t.Error("Expected error for 500 response, got nil")
	}
}

func TestClient_GetFillerLists_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetFillerLists(context.Background())
	if err == nil {
		t.Error("Expected error for 500 response, got nil")
	}
}

func TestClient_GetFillerContent_EmptyID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Should not reach server due to empty filler ID")
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetFillerContent(context.Background(), "")
	if err == nil {
		t.Error("Expected error for empty filler list ID, got nil")
	}
}

func TestClient_GetFillerContent_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetFillerContent(context.Background(), "filler-999")
	if err == nil {
		t.Error("Expected error for 404 response, got nil")
	}
}

func TestValidateProgram(t *testing.T) {
	tests := []struct {
		name    string
		program Program
		wantErr bool
	}{
		{
			name: "Valid movie",
			program: Program{
				ID:       "prog-1",
				Title:    "Test Movie",
				Duration: 7200000,
				Type:     "movie",
			},
			wantErr: false,
		},
		{
			name: "Valid episode",
			program: Program{
				ID:       "prog-2",
				Title:    "Test Episode",
				Duration: 1800000,
				Type:     "episode",
			},
			wantErr: false,
		},
		{
			name: "Valid track",
			program: Program{
				ID:       "prog-3",
				Title:    "Test Track",
				Duration: 30000,
				Type:     "track",
			},
			wantErr: false,
		},
		{
			name: "Empty type is valid",
			program: Program{
				ID:       "prog-4",
				Title:    "Test Program",
				Duration: 1800000,
				Type:     "",
			},
			wantErr: false,
		},
		{
			name: "Missing ID is valid (UUID can be used instead)",
			program: Program{
				ID:       "",
				Title:    "Test",
				Duration: 1800000,
				Type:     "movie",
			},
			wantErr: false, // ID is optional - programs can use UUID instead
		},
		{
			name: "Missing title",
			program: Program{
				ID:       "prog-5",
				Title:    "",
				Duration: 1800000,
				Type:     "movie",
			},
			wantErr: true,
		},
		{
			name: "Zero duration",
			program: Program{
				ID:       "prog-6",
				Title:    "Test",
				Duration: 0, // Zero duration is valid (for placeholder content)
				Type:     "movie",
			},
			wantErr: false,
		},
		{
			name: "Negative duration",
			program: Program{
				ID:       "prog-7",
				Title:    "Test",
				Duration: -100,
				Type:     "movie",
			},
			wantErr: true,
		},
		{
			name: "Invalid type",
			program: Program{
				ID:       "prog-8",
				Title:    "Test",
				Duration: 1800000,
				Type:     "invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test validation directly using httpclient.Validate
			// which is the same validation used internally
			err := httpclient.Validate(&tt.program)

			if tt.wantErr && err == nil {
				t.Error("Expected validation error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Expected no error, got %v", err)
			}
		})
	}
}
