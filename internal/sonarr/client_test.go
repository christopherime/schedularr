package sonarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetSeries(t *testing.T) {
	mockSeries := []Series{
		{ID: 10, Title: "Show A", Year: 2021},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v3/series" {
			t.Errorf("expected /api/v3/series path, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "sonarr-key" {
			t.Errorf("expected X-Api-Key header, got %s", r.Header.Get("X-Api-Key"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockSeries)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "sonarr-key"})
	series, err := client.GetSeries(context.Background())
	if err != nil {
		t.Fatalf("GetSeries returned error: %v", err)
	}

	if len(series) != len(mockSeries) {
		t.Fatalf("expected %d series, got %d", len(mockSeries), len(series))
	}
}

func TestClient_GetEpisodes(t *testing.T) {
	mockEpisodes := []Episode{
		{ID: 20, SeriesID: 10, Title: "Pilot", SeasonNumber: 1, EpisodeNumber: 1},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v3/episode" {
			t.Errorf("expected /api/v3/episode path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("seriesId") != "10" {
			t.Errorf("expected seriesId query, got %s", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockEpisodes)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	episodes, err := client.GetEpisodes(context.Background(), 10)
	if err != nil {
		t.Fatalf("GetEpisodes returned error: %v", err)
	}

	if len(episodes) != len(mockEpisodes) {
		t.Fatalf("expected %d episodes, got %d", len(mockEpisodes), len(episodes))
	}
}

func TestEpisodesToPrograms(t *testing.T) {
	seriesByID := map[int]Series{
		10: {ID: 10, Title: "Show A", Year: 2021, Runtime: 45, Genres: []string{"Drama"}},
	}
	episodes := []Episode{
		{ID: 20, SeriesID: 10, Title: "Pilot", SeasonNumber: 1, EpisodeNumber: 1, Runtime: 42, HasFile: true},
		{ID: 21, SeriesID: 10, Title: "Missing", SeasonNumber: 1, EpisodeNumber: 2, Runtime: 42, HasFile: false},
	}

	programs := EpisodesToPrograms(seriesByID, episodes)
	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}
	if programs[0].ShowTitle != "Show A" {
		t.Errorf("expected Show A, got %s", programs[0].ShowTitle)
	}
}
