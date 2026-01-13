package radarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_GetMovies(t *testing.T) {
	mockMovies := []Movie{
		{ID: 1, Title: "Movie A", Year: 2024, Runtime: 120, HasFile: true},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/api/v3/movie" {
			t.Errorf("expected /api/v3/movie path, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "radarr-key" {
			t.Errorf("expected X-Api-Key header, got %s", r.Header.Get("X-Api-Key"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(mockMovies)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "radarr-key"})
	movies, err := client.GetMovies(context.Background())
	if err != nil {
		t.Fatalf("GetMovies returned error: %v", err)
	}

	if len(movies) != len(mockMovies) {
		t.Fatalf("expected %d movies, got %d", len(mockMovies), len(movies))
	}
}

func TestClient_GetMovies_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-key"})
	_, err := client.GetMovies(context.Background())
	if err == nil {
		t.Error("expected error for 500 response, got nil")
	}
}

func TestClient_InvalidURL(t *testing.T) {
	client := NewClient(Config{URL: "://invalid-url", APIKey: "test-key"})
	_, err := client.GetMovies(context.Background())
	if err == nil {
		t.Error("expected error for invalid URL, got nil")
	}
}

