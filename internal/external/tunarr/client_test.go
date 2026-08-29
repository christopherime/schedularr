package tunarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/christopherime/schedularr/internal/httpclient"
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

func TestClient_UpdateSchedule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT request, got %s", r.Method)
		}
		if r.URL.Path != "/api/channels/channel-1/programming" {
			t.Errorf("expected /api/channels/channel-1/programming path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	schedule := []Program{
		{ID: "prog-1", Title: "Show A", Duration: 1800000, Type: "episode"},
	}
	err := client.UpdateSchedule(context.Background(), "channel-1", schedule)
	if err != nil {
		t.Fatalf("UpdateSchedule returned error: %v", err)
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

func TestClient_SearchPrograms(t *testing.T) {
	query := "Star"
	mockResponse := ProgramSearchResponse{
		Results: []Program{
			{ID: "prog-1", Title: "Star Wars", Duration: 7200000, Type: "movie"},
			{ID: "prog-2", Title: "Star Trek", Duration: 6000000, Type: "movie"},
		},
		Page:       1,
		TotalPages: 1,
		TotalHits:  2,
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
	// Schedule with invalid program (missing required title)
	schedule := []Program{
		{ID: "prog-1", Title: "", Duration: 1800000, Type: "movie"}, // Missing required title
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

func TestClient_SearchPrograms_EmptyQuery(t *testing.T) {
	// Note: The current implementation doesn't validate empty queries client-side.
	// The search will proceed and return results based on filters.
	// This test validates that the API call completes without error.
	mockResponse := ProgramSearchResponse{
		Results:    []Program{},
		Page:       1,
		TotalPages: 0,
		TotalHits:  0,
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

// TestHydrateEpisodeShowFields is a direct, HTTP-free unit test of
// hydrateEpisodeShowFields -- the choke point SearchPrograms and
// GetFillerPrograms both run their results through (see its doc comment
// in client.go). Covers every branch: nested show fills empty flat
// fields; an already-set flat field is never overridden; a program with
// no Show object is untouched; a non-episode Type is never hydrated even
// if it somehow carries a Show object.
func TestHydrateEpisodeShowFields(t *testing.T) {
	tests := []struct {
		name          string
		in            Program
		wantShowTitle string
		wantRating    string
	}{
		{
			name:          "episode with nested show and empty flats gets hydrated",
			in:            Program{Type: "episode", Show: &Show{Title: "The Office", Rating: "TV-14"}},
			wantShowTitle: "The Office",
			wantRating:    "TV-14",
		},
		{
			name: "episode with flat fields already set is left untouched",
			in: Program{
				Type: "episode", ShowTitle: "Flat Show", Rating: "PG",
				Show: &Show{Title: "Nested Show", Rating: "R"},
			},
			wantShowTitle: "Flat Show",
			wantRating:    "PG",
		},
		{
			name:          "episode with no show object is left untouched",
			in:            Program{Type: "episode"},
			wantShowTitle: "",
			wantRating:    "",
		},
		{
			name:          "movie with a show object (shouldn't happen live) is not hydrated",
			in:            Program{Type: "movie", Show: &Show{Title: "Should Not Apply", Rating: "PG-13"}},
			wantShowTitle: "",
			wantRating:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			programs := []Program{tt.in}
			hydrateEpisodeShowFields(programs)
			if programs[0].ShowTitle != tt.wantShowTitle {
				t.Errorf("ShowTitle = %q, want %q", programs[0].ShowTitle, tt.wantShowTitle)
			}
			if programs[0].Rating != tt.wantRating {
				t.Errorf("Rating = %q, want %q", programs[0].Rating, tt.wantRating)
			}
		})
	}
}

// TestClient_SearchPrograms_LiveNestedShowShape_HydratesShowTitleAndRating
// decodes a response body shaped exactly like a real Tunarr 1.3.13 POST
// /api/programs/search reply (live-verified this session; corroborated by
// docs/tunarr/openapi.json's response envelope and Episode/Show schemas):
// the envelope uses "page"/"totalPages"/"totalHits" (no legacy
// "total"/"limit"), and the episode result carries no flat "showTitle" or
// "rating" key at all -- only a nested "show" object. This is the pin for
// Client.SearchPrograms actually decoding and hydrating that live shape,
// not just a Go-struct round trip through this package's own marshaling.
func TestClient_SearchPrograms_LiveNestedShowShape_HydratesShowTitleAndRating(t *testing.T) {
	const body = `{
		"results": [
			{
				"uuid": "11111111-1111-1111-1111-111111111111",
				"type": "episode",
				"title": "Pilot",
				"duration": 1320000,
				"episodeNumber": 1,
				"show": {
					"uuid": "22222222-2222-2222-2222-222222222222",
					"type": "show",
					"title": "The Office",
					"rating": "TV-14"
				}
			}
		],
		"page": 1,
		"totalPages": 1,
		"totalHits": 1,
		"facetDistribution": {}
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("failed to write mock response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	resp, err := client.SearchPrograms(context.Background(), ProgramSearchRequest{Query: &ProgramSearchQuery{}})
	if err != nil {
		t.Fatalf("SearchPrograms returned error: %v", err)
	}

	if resp.Page != 1 || resp.TotalPages != 1 || resp.TotalHits != 1 {
		t.Errorf("expected envelope page=1 totalPages=1 totalHits=1, got page=%d totalPages=%d totalHits=%d",
			resp.Page, resp.TotalPages, resp.TotalHits)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}

	got := resp.Results[0]
	if got.ShowTitle != "The Office" {
		t.Errorf("expected hydrated ShowTitle %q, got %q", "The Office", got.ShowTitle)
	}
	if got.Rating != "TV-14" {
		t.Errorf("expected hydrated Rating %q, got %q", "TV-14", got.Rating)
	}
	if got.Show == nil || got.Show.Title != "The Office" {
		t.Errorf("expected Show to be preserved with Title %q, got %+v", "The Office", got.Show)
	}
}

// TestClient_SearchPrograms_HydrationDoesNotOverrideFlatShowTitle pins the
// fixture-compat path: a result that already carries flat showTitle/rating
// keys (this package's own README-documented fixture format,
// testdata/programs/*.json) must round-trip unchanged even if it also
// happens to carry a (differently-valued) nested "show" object -- hydration
// only ever fills an empty field, never overrides one a caller/fixture
// already set.
func TestClient_SearchPrograms_HydrationDoesNotOverrideFlatShowTitle(t *testing.T) {
	mockResponse := ProgramSearchResponse{
		Results: []Program{
			{
				ID: "ep-1", Title: "Diversity Day", Type: "episode", Duration: 1320000,
				ShowTitle: "The Office (flat)",
				Rating:    "TV-PG",
				Show:      &Show{UUID: "22222222-2222-2222-2222-222222222222", Title: "The Office (nested)", Rating: "TV-14"},
			},
		},
		Page: 1, TotalPages: 1, TotalHits: 1,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mockResponse); err != nil {
			t.Fatalf("failed to encode mock response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	resp, err := client.SearchPrograms(context.Background(), ProgramSearchRequest{Query: &ProgramSearchQuery{}})
	if err != nil {
		t.Fatalf("SearchPrograms returned error: %v", err)
	}

	got := resp.Results[0]
	if got.ShowTitle != "The Office (flat)" {
		t.Errorf("hydration must not override an already-set flat ShowTitle, got %q", got.ShowTitle)
	}
	if got.Rating != "TV-PG" {
		t.Errorf("hydration must not override an already-set flat Rating, got %q", got.Rating)
	}
}

// TestClient_GetFillerPrograms_HydratesNestedShow proves GetFillerPrograms
// runs its results through the same hydrateEpisodeShowFields choke point
// SearchPrograms does -- filler content is fetched from a different
// endpoint (GET /api/filler-lists/{id}/programs) but a live Tunarr nests
// show data the same way for any episode result.
func TestClient_GetFillerPrograms_HydratesNestedShow(t *testing.T) {
	mockPrograms := []Program{
		{
			ID: "ep-1", Title: "Pilot", Type: "episode", Duration: 1320000,
			Show: &Show{UUID: "33333333-3333-3333-3333-333333333333", Title: "Parks and Recreation", Rating: "TV-PG"},
		},
	}
	fillerListID := "filler-1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/filler-lists/" + fillerListID + "/programs"
		if r.URL.Path != expectedPath {
			t.Errorf("expected %s path, got %s", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mockPrograms); err != nil {
			t.Fatalf("failed to encode mock response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	programs, err := client.GetFillerPrograms(context.Background(), fillerListID)
	if err != nil {
		t.Fatalf("GetFillerPrograms returned error: %v", err)
	}

	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}
	if programs[0].ShowTitle != "Parks and Recreation" {
		t.Errorf("expected hydrated ShowTitle %q, got %q", "Parks and Recreation", programs[0].ShowTitle)
	}
	if programs[0].Rating != "TV-PG" {
		t.Errorf("expected hydrated Rating %q, got %q", "TV-PG", programs[0].Rating)
	}
}
