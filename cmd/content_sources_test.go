package cmd

import (
	"testing"

	"github.com/christopherime/schedularr/internal/external/radarr"
	"github.com/christopherime/schedularr/internal/external/sonarr"
	"github.com/christopherime/schedularr/internal/external/tunarr"
)

func TestNormalizeTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{"lowercase", "The Matrix", "the matrix"},
		{"trim spaces", "  Star Wars  ", "star wars"},
		{"already normalized", "inception", "inception"},
		{"mixed case with spaces", "  The Lord of the Rings  ", "the lord of the rings"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeTitle(tt.title)
			if got != tt.want {
				t.Errorf("normalizeTitle(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestMovieKey(t *testing.T) {
	tests := []struct {
		name  string
		title string
		year  int
		want  string
	}{
		{"basic", "The Matrix", 1999, "the matrix|1999"},
		{"with year 2000", "Gladiator", 2000, "gladiator|2000"},
		{"zero year", "Unknown", 0, "unknown|0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := movieKey(tt.title, tt.year)
			if got != tt.want {
				t.Errorf("movieKey(%q, %d) = %q, want %q", tt.title, tt.year, got, tt.want)
			}
		})
	}
}

func TestEpisodeKey(t *testing.T) {
	tests := []struct {
		name      string
		showTitle string
		season    int
		episode   int
		want      string
	}{
		{"basic", "Breaking Bad", 1, 1, "breaking bad|s1e1"},
		{"high numbers", "The Simpsons", 35, 22, "the simpsons|s35e22"},
		{"double digits", "Game of Thrones", 8, 6, "game of thrones|s8e6"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := episodeKey(tt.showTitle, tt.season, tt.episode)
			if got != tt.want {
				t.Errorf("episodeKey(%q, %d, %d) = %q, want %q", tt.showTitle, tt.season, tt.episode, got, tt.want)
			}
		})
	}
}

func TestBuildRadarrMovieIndex(t *testing.T) {
	movies := []radarr.Movie{
		{ID: 1, Title: "The Matrix", Year: 1999, HasFile: true},
		{ID: 2, Title: "Inception", Year: 2010, HasFile: true},
		{ID: 3, Title: "Missing Movie", Year: 2020, HasFile: false},
		{ID: 4, Title: "", Year: 2021, HasFile: true}, // Empty title
	}

	t.Run("exclude missing files", func(t *testing.T) {
		index := buildRadarrMovieIndex(movies, true)

		// Should have 2 movies (Matrix and Inception)
		if len(index.byTitle) != 2 {
			t.Errorf("expected 2 titles in index, got %d", len(index.byTitle))
		}
		if len(index.byTitleYear) != 2 {
			t.Errorf("expected 2 title+year combos in index, got %d", len(index.byTitleYear))
		}

		// Check specific entries
		if _, ok := index.byTitle["the matrix"]; !ok {
			t.Error("expected 'the matrix' in byTitle index")
		}
		if _, ok := index.byTitle["missing movie"]; ok {
			t.Error("'missing movie' should not be in index when excluding missing files")
		}
	})

	t.Run("include missing files", func(t *testing.T) {
		index := buildRadarrMovieIndex(movies, false)

		// Should have 3 movies (excludes empty title)
		if len(index.byTitle) != 3 {
			t.Errorf("expected 3 titles in index, got %d", len(index.byTitle))
		}

		if _, ok := index.byTitle["missing movie"]; !ok {
			t.Error("expected 'missing movie' in byTitle index when not excluding missing files")
		}
	})

	t.Run("empty movies list", func(t *testing.T) {
		index := buildRadarrMovieIndex([]radarr.Movie{}, true)

		if len(index.byTitle) != 0 {
			t.Errorf("expected empty byTitle index, got %d", len(index.byTitle))
		}
		if len(index.byTitleYear) != 0 {
			t.Errorf("expected empty byTitleYear index, got %d", len(index.byTitleYear))
		}
	})
}

func intPtr(i int) *int {
	return &i
}

func TestIsRadarrMovieAvailable(t *testing.T) {
	index := radarrMovieIndex{
		byTitle:     map[string]struct{}{"the matrix": {}, "inception": {}},
		byTitleYear: map[string]struct{}{"the matrix|1999": {}, "inception|2010": {}},
	}

	tests := []struct {
		name    string
		program tunarr.Program
		want    bool
	}{
		{
			name:    "exact match with year",
			program: tunarr.Program{Title: "The Matrix", Year: intPtr(1999), Type: "movie"},
			want:    true,
		},
		{
			name:    "title match without year",
			program: tunarr.Program{Title: "Inception", Type: "movie"},
			want:    true,
		},
		{
			name:    "title match with wrong year falls back to title",
			program: tunarr.Program{Title: "The Matrix", Year: intPtr(2022), Type: "movie"},
			want:    true, // Falls back to title-only match
		},
		{
			name:    "not in index",
			program: tunarr.Program{Title: "Avatar", Year: intPtr(2009), Type: "movie"},
			want:    false,
		},
		{
			name:    "empty title",
			program: tunarr.Program{Title: "", Year: intPtr(2020), Type: "movie"},
			want:    false,
		},
		{
			name:    "case insensitive match",
			program: tunarr.Program{Title: "THE MATRIX", Year: intPtr(1999), Type: "movie"},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRadarrMovieAvailable(tt.program, index)
			if got != tt.want {
				t.Errorf("isRadarrMovieAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterMoviePrograms(t *testing.T) {
	index := radarrMovieIndex{
		byTitle:     map[string]struct{}{"the matrix": {}, "inception": {}},
		byTitleYear: map[string]struct{}{"the matrix|1999": {}, "inception|2010": {}},
	}

	programs := []tunarr.Program{
		{ID: "1", Title: "The Matrix", Year: intPtr(1999), Type: "movie"},
		{ID: "2", Title: "Avatar", Year: intPtr(2009), Type: "movie"},
		{ID: "3", Title: "Inception", Year: intPtr(2010), Type: "movie"},
		{ID: "4", Title: "Breaking Bad", SeasonNumber: 1, EpisodeNumber: 1, Type: "episode"},
		{ID: "5", Title: "Unknown Movie", Year: intPtr(2020), Type: "movie"},
	}

	filtered, originalCount, filteredCount := filterMoviePrograms(programs, index)

	// Should have 4 programs: 2 available movies + 1 episode (untouched) + filter removes 2 movies
	if len(filtered) != 3 {
		t.Errorf("expected 3 filtered programs, got %d", len(filtered))
	}

	if originalCount != 4 {
		t.Errorf("expected originalCount = 4, got %d", originalCount)
	}

	if filteredCount != 2 {
		t.Errorf("expected filteredCount = 2, got %d", filteredCount)
	}

	// Verify episode is preserved
	hasEpisode := false
	for _, p := range filtered {
		if p.Type == "episode" {
			hasEpisode = true
			break
		}
	}
	if !hasEpisode {
		t.Error("episode should be preserved in filtered results")
	}
}

func TestBuildSeriesMap(t *testing.T) {
	series := []sonarr.Series{
		{ID: 1, Title: "Breaking Bad"},
		{ID: 2, Title: "The Wire"},
		{ID: 3, Title: "Game of Thrones"},
	}

	seriesMap := buildSeriesMap(series)

	if len(seriesMap) != 3 {
		t.Errorf("expected 3 series in map, got %d", len(seriesMap))
	}

	if s, ok := seriesMap[1]; !ok || s.Title != "Breaking Bad" {
		t.Error("expected series ID 1 to be 'Breaking Bad'")
	}

	if _, ok := seriesMap[999]; ok {
		t.Error("series ID 999 should not exist in map")
	}
}

func TestBuildSonarrEpisodeIndex(t *testing.T) {
	seriesByID := map[int]sonarr.Series{
		1: {ID: 1, Title: "Breaking Bad"},
		2: {ID: 2, Title: "The Wire"},
	}

	episodes := []sonarr.Episode{
		{ID: 1, SeriesID: 1, SeasonNumber: 1, EpisodeNumber: 1, HasFile: true},
		{ID: 2, SeriesID: 1, SeasonNumber: 1, EpisodeNumber: 2, HasFile: true},
		{ID: 3, SeriesID: 1, SeasonNumber: 1, EpisodeNumber: 3, HasFile: false}, // Missing
		{ID: 4, SeriesID: 2, SeasonNumber: 1, EpisodeNumber: 1, HasFile: true},
		{ID: 5, SeriesID: 999, SeasonNumber: 1, EpisodeNumber: 1, HasFile: true}, // Unknown series
	}

	t.Run("exclude missing files", func(t *testing.T) {
		index := buildSonarrEpisodeIndex(seriesByID, episodes, true)

		// Should have 3 episodes (excludes missing and unknown series)
		if len(index) != 3 {
			t.Errorf("expected 3 episodes in index, got %d", len(index))
		}

		if _, ok := index["breaking bad|s1e1"]; !ok {
			t.Error("expected 'breaking bad|s1e1' in index")
		}
		if _, ok := index["breaking bad|s1e3"]; ok {
			t.Error("'breaking bad|s1e3' should not be in index (missing file)")
		}
	})

	t.Run("include missing files", func(t *testing.T) {
		index := buildSonarrEpisodeIndex(seriesByID, episodes, false)

		// Should have 4 episodes (excludes only unknown series)
		if len(index) != 4 {
			t.Errorf("expected 4 episodes in index, got %d", len(index))
		}

		if _, ok := index["breaking bad|s1e3"]; !ok {
			t.Error("expected 'breaking bad|s1e3' in index when not excluding missing files")
		}
	})
}

func TestIsSonarrEpisodeAvailable(t *testing.T) {
	available := map[string]struct{}{
		"breaking bad|s1e1": {},
		"breaking bad|s1e2": {},
		"the wire|s1e1":     {},
	}

	tests := []struct {
		name    string
		program tunarr.Program
		want    bool
	}{
		{
			name: "available episode",
			program: tunarr.Program{
				Title:         "Breaking Bad S01E01",
				ShowTitle:     "Breaking Bad",
				SeasonNumber:  1,
				EpisodeNumber: 1,
				Type:          "episode",
			},
			want: true,
		},
		{
			name: "unavailable episode",
			program: tunarr.Program{
				Title:         "Breaking Bad S01E03",
				ShowTitle:     "Breaking Bad",
				SeasonNumber:  1,
				EpisodeNumber: 3,
				Type:          "episode",
			},
			want: false,
		},
		{
			name: "missing show title - passes through",
			program: tunarr.Program{
				Title:         "Some Episode",
				ShowTitle:     "",
				SeasonNumber:  1,
				EpisodeNumber: 1,
				Type:          "episode",
			},
			want: true, // Passes through when ShowTitle is empty
		},
		{
			name: "zero season number - passes through",
			program: tunarr.Program{
				Title:         "Some Episode",
				ShowTitle:     "Unknown Show",
				SeasonNumber:  0,
				EpisodeNumber: 1,
				Type:          "episode",
			},
			want: true, // Passes through when SeasonNumber is 0
		},
		{
			name: "zero episode number - passes through",
			program: tunarr.Program{
				Title:         "Some Episode",
				ShowTitle:     "Unknown Show",
				SeasonNumber:  1,
				EpisodeNumber: 0,
				Type:          "episode",
			},
			want: true, // Passes through when EpisodeNumber is 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSonarrEpisodeAvailable(tt.program, available)
			if got != tt.want {
				t.Errorf("isSonarrEpisodeAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterEpisodePrograms(t *testing.T) {
	available := map[string]struct{}{
		"breaking bad|s1e1": {},
		"breaking bad|s1e2": {},
	}

	programs := []tunarr.Program{
		{ID: "1", Title: "BB S01E01", ShowTitle: "Breaking Bad", SeasonNumber: 1, EpisodeNumber: 1, Type: "episode"},
		{ID: "2", Title: "BB S01E02", ShowTitle: "Breaking Bad", SeasonNumber: 1, EpisodeNumber: 2, Type: "episode"},
		{ID: "3", Title: "BB S01E03", ShowTitle: "Breaking Bad", SeasonNumber: 1, EpisodeNumber: 3, Type: "episode"},
		{ID: "4", Title: "The Matrix", Year: intPtr(1999), Type: "movie"}, // Should be preserved
	}

	filtered, originalCount, filteredCount := filterEpisodePrograms(programs, available)

	// Should have 3 programs: 2 available episodes + 1 movie (untouched)
	if len(filtered) != 3 {
		t.Errorf("expected 3 filtered programs, got %d", len(filtered))
	}

	if originalCount != 3 {
		t.Errorf("expected originalCount = 3, got %d", originalCount)
	}

	if filteredCount != 2 {
		t.Errorf("expected filteredCount = 2, got %d", filteredCount)
	}

	// Verify movie is preserved
	hasMovie := false
	for _, p := range filtered {
		if p.Type == "movie" {
			hasMovie = true
			break
		}
	}
	if !hasMovie {
		t.Error("movie should be preserved in filtered results")
	}
}

func TestFilterMoviePrograms_EmptyIndex(t *testing.T) {
	index := radarrMovieIndex{
		byTitle:     map[string]struct{}{},
		byTitleYear: map[string]struct{}{},
	}

	programs := []tunarr.Program{
		{ID: "1", Title: "The Matrix", Year: intPtr(1999), Type: "movie"},
		{ID: "2", Title: "Breaking Bad", Type: "episode"},
	}

	filtered, originalCount, filteredCount := filterMoviePrograms(programs, index)

	// Episode should be preserved, movie should be filtered out
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered program, got %d", len(filtered))
	}

	if originalCount != 1 {
		t.Errorf("expected originalCount = 1, got %d", originalCount)
	}

	if filteredCount != 0 {
		t.Errorf("expected filteredCount = 0, got %d", filteredCount)
	}
}

func TestFilterEpisodePrograms_EmptyAvailable(t *testing.T) {
	available := map[string]struct{}{}

	programs := []tunarr.Program{
		{ID: "1", Title: "BB S01E01", ShowTitle: "Breaking Bad", SeasonNumber: 1, EpisodeNumber: 1, Type: "episode"},
		{ID: "2", Title: "The Matrix", Type: "movie"},
	}

	filtered, originalCount, filteredCount := filterEpisodePrograms(programs, available)

	// Movie should be preserved, episode should be filtered out
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered program, got %d", len(filtered))
	}

	if originalCount != 1 {
		t.Errorf("expected originalCount = 1, got %d", originalCount)
	}

	if filteredCount != 0 {
		t.Errorf("expected filteredCount = 0, got %d", filteredCount)
	}
}
