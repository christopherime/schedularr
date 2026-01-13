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

func TestClient_GetSeries_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-key"})
	_, err := client.GetSeries(context.Background())
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestClient_GetEpisodes_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	_, err := client.GetEpisodes(context.Background(), 999)
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

func TestClient_InvalidURL(t *testing.T) {
	client := NewClient(Config{URL: "://invalid-url", APIKey: "test-key"})
	_, err := client.GetSeries(context.Background())
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}
