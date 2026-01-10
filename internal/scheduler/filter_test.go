package scheduler

import (
	"testing"

	"github.com/geekxflood/schedularr/internal/tunarr"
)

func TestFilterPrograms(t *testing.T) {
	programs := []tunarr.Program{
		{Title: "Movie A", Year: 2000, Genres: []string{"Action"}, Duration: 6000000}, // 100 min
		{Title: "Show B", Year: 2020, Genres: []string{"Comedy"}, Duration: 1800000}, // 30 min
	}

	f := Filter{
		Genres:      []string{"Action"},
		MinDuration: 90,
	}

	filtered, err := FilterPrograms(programs, f)
	if err != nil {
		t.Fatalf("FilterPrograms returned error: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("Expected 1 program, got %d", len(filtered))
	}
	if filtered[0].Title != "Movie A" {
		t.Errorf("Expected Movie A, got %s", filtered[0].Title)
	}
}
