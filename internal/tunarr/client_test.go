package tunarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestClient_GetChannels(t *testing.T) {
	// mock response data
	mockChannels := []Channel{
		{
			ID:      "channel-1",
			Number:  1,
			Name:    "Test Channel 1",
			Icon:    "http://example.com/icon1.png",
			Group:   "General",
			Enabled: true,
		},
		{
			ID:      "channel-2",
			Number:  2,
			Name:    "Test Channel 2",
			Icon:    "http://example.com/icon2.png",
			Group:   "Movies",
			Enabled: false,
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

func TestClient_GetPrograms(t *testing.T) {
	mockPrograms := []Program{
		{
			ID:       "prog-1",
			Title:    "Movie A",
			Year:     2023,
			Duration: 7200000,
			Type:     "movie",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/programs" {
			t.Errorf("expected /api/programs path, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(mockPrograms); err != nil {
			t.Fatalf("failed to encode mock response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	programs, err := client.GetPrograms(context.Background())
	if err != nil {
		t.Fatalf("GetPrograms returned error: %v", err)
	}

	if len(programs) != len(mockPrograms) {
		t.Errorf("expected %d programs, got %d", len(mockPrograms), len(programs))
	}
	if !reflect.DeepEqual(programs[0], mockPrograms[0]) {
		t.Errorf("program mismatch: expected %+v, got %+v", mockPrograms[0], programs[0])
	}
}

func TestClient_UpdateSchedule(t *testing.T) {
	channelID := "channel-1"
	schedule := []Program{
		{ID: "prog-1", Title: "Show A", Duration: 1800000, Type: "episode"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/api/channels/"+channelID+"/schedule" {
			t.Errorf("expected correct path, got %s", r.URL.Path)
		}

		var receivedSchedule []Program
		if err := json.NewDecoder(r.Body).Decode(&receivedSchedule); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		
		if len(receivedSchedule) != len(schedule) {
			t.Errorf("expected %d items in schedule, got %d", len(schedule), len(receivedSchedule))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	err := client.UpdateSchedule(context.Background(), channelID, schedule)
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
		{ID: "lib-1", Name: "Movies", Type: "movie", Server: "plex"},
		{ID: "lib-2", Name: "TV Shows", Type: "show", Server: "plex"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/libraries" {
			t.Errorf("expected /api/libraries path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockLibraries)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	libraries, err := client.GetLibraries(context.Background())
	if err != nil {
		t.Fatalf("GetLibraries returned error: %v", err)
	}

	if len(libraries) != len(mockLibraries) {
		t.Errorf("expected %d libraries, got %d", len(mockLibraries), len(libraries))
	}
}

func TestClient_GetLibraryPrograms(t *testing.T) {
	libraryID := "lib-1"
	mockPrograms := []Program{
		{ID: "prog-1", Title: "Movie A", Duration: 7200000, Type: "movie"},
		{ID: "prog-2", Title: "Movie B", Duration: 6000000, Type: "movie"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		expectedPath := "/api/libraries/" + libraryID + "/programs"
		if r.URL.Path != expectedPath {
			t.Errorf("expected %s path, got %s", expectedPath, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockPrograms)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	programs, err := client.GetLibraryPrograms(context.Background(), libraryID)
	if err != nil {
		t.Fatalf("GetLibraryPrograms returned error: %v", err)
	}

	if len(programs) != len(mockPrograms) {
		t.Errorf("expected %d programs, got %d", len(mockPrograms), len(programs))
	}
}

func TestClient_GetShows(t *testing.T) {
	mockShows := []Show{
		{ID: "show-1", Title: "Show A", Seasons: 5, Episodes: 100},
		{ID: "show-2", Title: "Show B", Seasons: 3, Episodes: 60},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/shows" {
			t.Errorf("expected /api/shows path, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockShows)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	shows, err := client.GetShows(context.Background())
	if err != nil {
		t.Fatalf("GetShows returned error: %v", err)
	}

	if len(shows) != len(mockShows) {
		t.Errorf("expected %d shows, got %d", len(mockShows), len(shows))
	}
}

func TestClient_GetShowEpisodes(t *testing.T) {
	showID := "show-1"
	season := 1
	mockEpisodes := []Program{
		{ID: "ep-1", Title: "Episode 1", Duration: 1800000, Type: "episode", Season: season, Episode: 1},
		{ID: "ep-2", Title: "Episode 2", Duration: 1800000, Type: "episode", Season: season, Episode: 2},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		expectedPath := "/api/shows/" + showID + "/episodes"
		if r.URL.Path != expectedPath {
			t.Errorf("expected %s path, got %s", expectedPath, r.URL.Path)
		}
		if r.URL.RawQuery != "season=1" {
			t.Errorf("expected season=1 query param, got %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockEpisodes)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	episodes, err := client.GetShowEpisodes(context.Background(), showID, season)
	if err != nil {
		t.Fatalf("GetShowEpisodes returned error: %v", err)
	}

	if len(episodes) != len(mockEpisodes) {
		t.Errorf("expected %d episodes, got %d", len(mockEpisodes), len(episodes))
	}
}

func TestClient_SearchPrograms(t *testing.T) {
	query := "Star"
	mockResults := []Program{
		{ID: "prog-1", Title: "Star Wars", Duration: 7200000, Type: "movie"},
		{ID: "prog-2", Title: "Star Trek", Duration: 6000000, Type: "movie"},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/programs/search" {
			t.Errorf("expected /api/programs/search path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("q") != query {
			t.Errorf("expected q=%s query param, got %s", query, r.URL.Query().Get("q"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockResults)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	programs, err := client.SearchPrograms(context.Background(), query)
	if err != nil {
		t.Fatalf("SearchPrograms returned error: %v", err)
	}

	if len(programs) != len(mockResults) {
		t.Errorf("expected %d programs, got %d", len(mockResults), len(programs))
	}
}

func TestClient_GetFillerLists(t *testing.T) {
	mockFillers := []FillerList{
		{ID: "filler-1", Name: "Commercials", Count: 50},
		{ID: "filler-2", Name: "Bumpers", Count: 30},
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
		expectedPath := "/api/filler-lists/" + fillerID + "/content"
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