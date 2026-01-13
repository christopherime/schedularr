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

func TestMoviesToPrograms(t *testing.T) {
	movies := []Movie{
		{ID: 1, Title: "Movie A", Year: 2024, Runtime: 120, HasFile: true},
		{ID: 2, Title: "Movie B", Year: 2023, Runtime: 0, HasFile: true},
		{ID: 3, Title: "Movie C", Year: 2022, Runtime: 90, HasFile: false},
	}

	programs := MoviesToPrograms(movies)
	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}
	if programs[0].Title != "Movie A" {
		t.Errorf("expected Movie A, got %s", programs[0].Title)
	}
	if programs[0].Duration <= 0 {
		t.Errorf("expected duration to be set")
	}
}
