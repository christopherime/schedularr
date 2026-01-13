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

func TestMoviesToPrograms_EmptyInput(t *testing.T) {
	movies := []Movie{}
	programs := MoviesToPrograms(movies)
	if len(programs) != 0 {
		t.Errorf("expected 0 programs for empty input, got %d", len(programs))
	}
}

func TestMoviesToPrograms_MultipleMovies(t *testing.T) {
	movies := []Movie{
		{ID: 1, Title: "Action Movie", Year: 2024, Runtime: 120, HasFile: true, Genres: []string{"Action"}},
		{ID: 2, Title: "Comedy Movie", Year: 2023, Runtime: 95, HasFile: true, Genres: []string{"Comedy"}},
		{ID: 3, Title: "Missing File", Year: 2022, Runtime: 110, HasFile: false},
		{ID: 4, Title: "No Runtime", Year: 2021, Runtime: 0, HasFile: true},
	}

	programs := MoviesToPrograms(movies)
	if len(programs) != 2 {
		t.Fatalf("expected 2 programs (with file and runtime), got %d", len(programs))
	}

	// Verify correct movies were converted
	titles := make(map[string]bool)
	for _, prog := range programs {
		titles[prog.Title] = true
	}

	if !titles["Action Movie"] {
		t.Error("expected Action Movie in programs")
	}
	if !titles["Comedy Movie"] {
		t.Error("expected Comedy Movie in programs")
	}
	if titles["Missing File"] {
		t.Error("did not expect Missing File in programs")
	}
	if titles["No Runtime"] {
		t.Error("did not expect No Runtime in programs")
	}
}

func TestMoviesToPrograms_GenresAndRatings(t *testing.T) {
	movies := []Movie{
		{
			ID:            1,
			Title:         "Test Movie",
			Year:          2024,
			Runtime:       120,
			HasFile:       true,
			Genres:        []string{"Action", "Thriller"},
			Certification: "PG-13",
		},
	}

	programs := MoviesToPrograms(movies)
	if len(programs) != 1 {
		t.Fatalf("expected 1 program, got %d", len(programs))
	}

	prog := programs[0]
	if len(prog.Genres) != 2 {
		t.Errorf("expected 2 genres, got %d", len(prog.Genres))
	}
	if prog.Rating != "PG-13" {
		t.Errorf("expected PG-13 rating, got %s", prog.Rating)
	}
	if prog.Type != "movie" {
		t.Errorf("expected type 'movie', got %s", prog.Type)
	}
}
