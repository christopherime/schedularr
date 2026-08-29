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

// TestClient_UpdateSchedule pins the live v1.3.13 wire contract for
// UpdateSchedule (see ManualLineupRequest's doc comment in models.go for
// how it was verified): POST (not PUT) to
// /api/channels/{id}/programming, body {"type": "manual", "lineup":
// [{"type": "content", "id": ..., "duration": ...}, ...], "append":
// false}. The server here decodes the raw body rather than trusting
// Client's own request-building, so a regression that reverts to the old
// PUT-with-a-bare-[]Program-body shape fails this test on the body
// assertions even if it somehow still hit this path.
func TestClient_UpdateSchedule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/channels/channel-1/programming" {
			t.Errorf("expected /api/channels/channel-1/programming path, got %s", r.URL.Path)
		}

		var body ManualLineupRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if body.Type != "manual" {
			t.Errorf("expected type %q, got %q", "manual", body.Type)
		}
		if body.Append {
			t.Error("expected append=false for a full-replacement UpdateSchedule call")
		}
		if len(body.Lineup) != 1 {
			t.Fatalf("expected 1 lineup entry, got %d: %+v", len(body.Lineup), body.Lineup)
		}
		item := body.Lineup[0]
		if item.Type != "content" {
			t.Errorf("expected lineup entry type %q, got %q", "content", item.Type)
		}
		if item.ID != "prog-1" {
			t.Errorf("expected lineup entry id %q, got %q", "prog-1", item.ID)
		}
		if item.Duration != 1800000 {
			t.Errorf("expected lineup entry duration 1800000, got %v", item.Duration)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
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

// TestClient_UpdateSchedule_NeverSendsPUT is the regression test for the
// bug this round fixes: a live Tunarr 1.3.13 instance has no PUT route
// for /api/channels/{id}/programming at all (live-verified this session:
// PUT returns 404 "Route PUT:... not found"; POST with an empty body
// returns 400, proving the route exists). This fake mirrors exactly that
// live behavior -- PUT is 404, POST succeeds -- so a regression back to
// PUT fails here with the same error a real deployment would hit, instead
// of silently passing against a fake that's more lenient than reality.
func TestClient_UpdateSchedule_NeverSendsPUT(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "Route PUT:/api/channels/channel-1/programming not found"})
			return
		}
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	schedule := []Program{
		{ID: "prog-1", Title: "Show A", Duration: 1800000, Type: "episode"},
	}
	err := client.UpdateSchedule(context.Background(), "channel-1", schedule)
	if err != nil {
		t.Fatalf("UpdateSchedule returned error: %v -- client must send POST, a PUT would 404 against a live Tunarr 1.3.13 instance", err)
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
		// The following (season/album/artist/collection/folder/playlist)
		// were all MISSING from Type's oneof list until this round --
		// live-verified this session that a live /api/programs/search
		// result can carry any of them (season, specifically: a growing
		// library scan started interleaving Type == "season" entries,
		// which validateProgram rejected outright, and the pre-fix caller
		// treated that single rejection as fatal to the ENTIRE fetch --
		// see filterValidPrograms' doc comment in client.go for the
		// skip-and-continue half of that fix). This table pins that the
		// oneof itself is now complete, independent of the
		// skip-and-continue behavior.
		{
			name: "Valid season",
			program: Program{
				UUID:  "season-1",
				Title: "Season 1",
				Type:  "season",
			},
			wantErr: false,
		},
		{
			name: "Valid album",
			program: Program{
				UUID:  "album-1",
				Title: "Test Album",
				Type:  "album",
			},
			wantErr: false,
		},
		{
			name: "Valid artist",
			program: Program{
				UUID:  "artist-1",
				Title: "Test Artist",
				Type:  "artist",
			},
			wantErr: false,
		},
		{
			name: "Valid collection",
			program: Program{
				UUID:  "collection-1",
				Title: "Test Collection",
				Type:  "collection",
			},
			wantErr: false,
		},
		{
			name: "Valid folder",
			program: Program{
				UUID:  "folder-1",
				Title: "Test Folder",
				Type:  "folder",
			},
			wantErr: false,
		},
		{
			name: "Valid playlist",
			program: Program{
				UUID:  "playlist-1",
				Title: "Test Playlist",
				Type:  "playlist",
			},
			wantErr: false,
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

// TestClient_SearchPrograms_NestedShowShape_HydratesShowTitleAndRating
// decodes a raw response body -- not a Go-struct round trip through this
// package's own marshaling -- to pin two things at once:
//
//  1. The response ENVELOPE ("page"/"totalPages"/"totalHits", no legacy
//     "total"/"limit") -- live-verified this session against a real
//     Tunarr 1.3.13 instance (transcript in this task's report) and
//     corroborated by docs/tunarr/openapi.json. This part of the shape is
//     accurate.
//  2. hydrateEpisodeShowFields's SECONDARY, defensive hydration path (an
//     episode result nesting a "show" object). This part of the shape is
//     NOT what live Tunarr sends -- live-verified this session that a
//     real episode result carries no nested "show" object at all, only a
//     "showId" foreign key (see Program.ShowTitle's doc comment in
//     models.go, and
//     TestClient_SearchPrograms_LiveShape_DecodesShowIDForeignKey below
//     for the actual live-shaped test). A prior round of this fix
//     claimed this nested shape was live-accurate; that claim was wrong.
//     Kept because the nested-Show path itself is still real,
//     intentionally-retained code.
func TestClient_SearchPrograms_NestedShowShape_HydratesShowTitleAndRating(t *testing.T) {
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
// endpoint (GET /api/filler-lists/{id}/programs), but the same secondary,
// defensive nested-Show hydration path applies there too. (This pins the
// secondary path specifically, not a live-shaped claim -- see
// Program.ShowTitle's doc comment in models.go: a live episode result
// never nests a "show" object.)
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
	programs, dropped, err := client.GetFillerPrograms(context.Background(), fillerListID)
	if err != nil {
		t.Fatalf("GetFillerPrograms returned error: %v", err)
	}
	if dropped != 0 {
		t.Errorf("expected 0 dropped programs, got %d", dropped)
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

// TestClient_SearchPrograms_LiveShape_DecodesShowIDForeignKey decodes a
// response body shaped exactly like what a real Tunarr 1.3.13
// /api/programs/search reply actually contains -- live-verified this
// session (transcript in this task's report): an episode entry with only
// a "showId" foreign key (no "showTitle", no "rating", no nested "show"
// object at all), and a SEPARATE, interleaved Type == "show" entry
// related only by "uuid", not nesting.
//
// Client.SearchPrograms has no join logic of its own (see
// hydrateEpisodeShowFields's doc comment) -- that's
// service.Runner.hydrateShowTitleAndRating's job, over the fully
// accumulated result set (see schedule_test.go in internal/service for
// the join itself). This test only pins that the DECODE is faithful to
// the live shape: ShowID/SeasonID populate from the flat keys, Show stays
// nil, and ShowTitle/Rating stay empty -- nothing is (or could be)
// hydrated at this layer.
func TestClient_SearchPrograms_LiveShape_DecodesShowIDForeignKey(t *testing.T) {
	const body = `{
		"results": [
			{
				"uuid": "55555555-5555-5555-5555-555555555555",
				"type": "show",
				"title": "The Office",
				"rating": "TV-14"
			},
			{
				"uuid": "11111111-1111-1111-1111-111111111111",
				"type": "episode",
				"title": "Pilot",
				"duration": 1320000,
				"episodeNumber": 1,
				"showId": "55555555-5555-5555-5555-555555555555",
				"seasonId": "66666666-6666-6666-6666-666666666666"
			}
		],
		"page": 1,
		"totalPages": 1,
		"totalHits": 2,
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
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}

	var episode *Program
	for i := range resp.Results {
		if resp.Results[i].Type == "episode" {
			episode = &resp.Results[i]
		}
	}
	if episode == nil {
		t.Fatal("expected an episode result")
	}

	if episode.ShowID != "55555555-5555-5555-5555-555555555555" {
		t.Errorf("expected ShowID decoded from the flat \"showId\" key, got %q", episode.ShowID)
	}
	if episode.SeasonID != "66666666-6666-6666-6666-666666666666" {
		t.Errorf("expected SeasonID decoded from the flat \"seasonId\" key, got %q", episode.SeasonID)
	}
	if episode.Show != nil {
		t.Errorf("expected Show to stay nil -- a live episode never nests one, got %+v", episode.Show)
	}
	if episode.ShowTitle != "" {
		t.Errorf("expected ShowTitle to stay empty at the client layer (joining is service.Runner's job), got %q", episode.ShowTitle)
	}
	if episode.Rating != "" {
		t.Errorf("expected Rating to stay empty at the client layer, got %q", episode.Rating)
	}
}

// TestClient_GetSeason exercises the happy path: GET
// /api/programming/seasons/{id} decoded into a Season, path built
// correctly from the season ID.
func TestClient_GetSeason(t *testing.T) {
	seasonID := "99051dca-8fdb-4f74-a315-f54a541ee261"
	mockSeason := Season{UUID: seasonID, Title: "Season 1", SeasonNumber: 1}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expectedPath := "/api/programming/seasons/" + seasonID
		if r.URL.Path != expectedPath {
			t.Errorf("expected %s path, got %s", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(mockSeason); err != nil {
			t.Fatalf("failed to encode mock response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	season, err := client.GetSeason(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("GetSeason returned error: %v", err)
	}
	if season.SeasonNumber != 1 {
		t.Errorf("expected SeasonNumber 1, got %d", season.SeasonNumber)
	}
	if season.Title != "Season 1" {
		t.Errorf("expected Title %q, got %q", "Season 1", season.Title)
	}
}

// TestClient_GetSeason_LiveIndexKey decodes a raw response body shaped
// exactly like a real GET /api/programming/seasons/{id} reply --
// live-verified this session (transcript in this task's report): the
// season number is under "index", not "seasonNumber". A prior version of
// the Season struct used the wrong tag and so never actually decoded this
// field from a real response.
func TestClient_GetSeason_LiveIndexKey(t *testing.T) {
	const body = `{
		"uuid": "99051dca-8fdb-4f74-a315-f54a541ee261",
		"title": "Saison 1",
		"index": 1,
		"type": "season"
	}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("failed to write mock response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	season, err := client.GetSeason(context.Background(), "99051dca-8fdb-4f74-a315-f54a541ee261")
	if err != nil {
		t.Fatalf("GetSeason returned error: %v", err)
	}
	if season.SeasonNumber != 1 {
		t.Errorf("expected SeasonNumber 1 decoded from \"index\", got %d", season.SeasonNumber)
	}
}

func TestClient_GetSeason_EmptyID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server with an empty season ID")
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetSeason(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty season ID, got nil")
	}
}

func TestClient_GetSeason_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetSeason(context.Background(), "some-id")
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

// TestClient_SearchPrograms_SkipsInvalidEntriesAndCountsDropped is the
// regression test for the pre-existing bug this round fixes: a live
// library search interleaves entries of several different Type values --
// live-verified this session (transcript in this task's report) that a
// growing library's search started returning Type == "season" entries
// (previously missing from the oneof, see TestValidateProgram) and that
// nothing rules out an entirely unrecognized future Type value either.
// Before this fix, ANY single invalid entry anywhere in a page made
// SearchPrograms return an error for the WHOLE page -- discarding every
// other, perfectly valid entry on it (and, transitively, every
// already-accumulated page in a paginated fetch; see
// internal/service/schedule.go's fetchSingleLibrary/
// fetchAllProgramsViaSearch). This response mixes a valid movie, a valid
// (now that the oneof is fixed) season entry, and a truly-unknown-type
// entry that must never be valid: SearchPrograms must still succeed,
// keep the two valid entries, and report DroppedCount == 1.
func TestClient_SearchPrograms_SkipsInvalidEntriesAndCountsDropped(t *testing.T) {
	mockResponse := ProgramSearchResponse{
		Results: []Program{
			{ID: "m1", Title: "A Movie", Type: "movie", Duration: 1_800_000},
			{UUID: "season-1", Title: "Season 1", Type: "season", Index: 1},
			{ID: "mystery-1", Title: "Mystery Entry", Type: "definitely-not-a-real-type", Duration: 100},
		},
		Page: 1, TotalPages: 1, TotalHits: 3,
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
		t.Fatalf("SearchPrograms returned error: %v -- a single invalid entry must never fail the whole page", err)
	}

	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 valid results (movie + season), got %d: %+v", len(resp.Results), resp.Results)
	}
	if resp.DroppedCount != 1 {
		t.Errorf("expected DroppedCount 1, got %d", resp.DroppedCount)
	}

	var haveMovie, haveSeason bool
	for _, p := range resp.Results {
		switch p.Type {
		case "movie":
			haveMovie = true
		case "season":
			haveSeason = true
			if p.Index != 1 {
				t.Errorf("expected season Index 1, got %d", p.Index)
			}
		}
	}
	if !haveMovie {
		t.Error("expected the valid movie to survive")
	}
	if !haveSeason {
		t.Error("expected the season entry to survive now that \"season\" is a valid Type")
	}
}
