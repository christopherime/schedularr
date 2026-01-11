package tunarr

import (
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
	channels, err := client.GetChannels()
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
	programs, err := client.GetPrograms()
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
		{ID: "prog-1", Title: "Show A"},
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
	err := client.UpdateSchedule(channelID, schedule)
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

	_, err := client.GetChannels()
	if err == nil {
		t.Error("expected error from non-200 status code, got nil")
	}
}