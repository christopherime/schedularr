package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/radarr"
	"github.com/geekxflood/schedularr/internal/sonarr"
	"github.com/geekxflood/schedularr/internal/tunarr"
)

func fetchAllContent(cfg *config.Config, client *tunarr.Client) ([]tunarr.Program, error) {
	fmt.Println(infoStyle.Render("📡 Fetching content from Tunarr..."))
	programs := fetchTunarrContent(client)

	if len(programs) == 0 {
		fmt.Println(warnStyle.Render("⚠ No content available - using fallback GetPrograms()"))
		var err error
		programs, err = client.GetPrograms(context.Background())
		if err != nil {
			return nil, fmt.Errorf("failed to fetch programs: %w", err)
		}
	}

	if cfg.Radarr.URL != "" {
		programs = applyRadarrAvailability(cfg, programs)
	}

	if cfg.Sonarr.URL != "" {
		programs = applySonarrAvailability(cfg, programs)
	}

	return programs, nil
}

func fetchTunarrContent(client *tunarr.Client) []tunarr.Program {
	var allPrograms []tunarr.Program

	libraries, err := client.GetLibraries(context.Background())
	if err != nil {
		if verbose {
			fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch libraries: %v", err)))
		}
		return allPrograms
	}

	if verbose {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📚 Found %d librar(y/ies)", len(libraries))))
	}

	for _, lib := range libraries {
		if verbose {
			fmt.Printf("  - %s (%s)\n", lib.Name, lib.Type)
		}

		programs, err := client.GetLibraryPrograms(context.Background(), lib.ID)
		if err != nil {
			if verbose {
				fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("    ⚠ Could not fetch programs from %s: %v", lib.Name, err)))
			}
			continue
		}

		if verbose {
			fmt.Printf("    ✓ %d programs\n", len(programs))
		}

		allPrograms = append(allPrograms, programs...)
	}

	return allPrograms
}

func applyRadarrAvailability(cfg *config.Config, programs []tunarr.Program) []tunarr.Program {
	client := radarr.NewClient(cfg.Radarr)
	movies, err := client.GetMovies(context.Background())
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch Radarr movies: %v", err)))
		return programs
	}

	availableTitles := buildRadarrMovieIndex(movies)
	if len(availableTitles.byTitle) == 0 && len(availableTitles.byTitleYear) == 0 {
		return programs
	}

	filtered, originalCount, filteredCount := filterMoviePrograms(programs, availableTitles)
	if originalCount > 0 && filteredCount == 0 {
		fmt.Printf("%s\n", warnStyle.Render("⚠ Radarr filtering removed all movie programs; keeping original list"))
		return programs
	}

	if verbose && filteredCount < originalCount {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🎞️  Radarr availability filtered %d movie(s)", originalCount-filteredCount)))
	}

	return filtered
}

func applySonarrAvailability(cfg *config.Config, programs []tunarr.Program) []tunarr.Program {
	client := sonarr.NewClient(cfg.Sonarr)
	series, err := client.GetSeries(context.Background())
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch Sonarr series: %v", err)))
		return programs
	}

	seriesByID := make(map[int]sonarr.Series, len(series))
	for _, s := range series {
		seriesByID[s.ID] = s
	}

	var episodes []sonarr.Episode
	for _, s := range series {
		seriesEpisodes, err := client.GetEpisodes(context.Background(), s.ID)
		if err != nil {
			if verbose {
				fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch Sonarr episodes for %s: %v", s.Title, err)))
			}
			continue
		}
		episodes = append(episodes, seriesEpisodes...)
	}

	availableEpisodes := buildSonarrEpisodeIndex(seriesByID, episodes)
	if len(availableEpisodes) == 0 {
		return programs
	}

	filtered, originalCount, filteredCount := filterEpisodePrograms(programs, availableEpisodes)
	if originalCount > 0 && filteredCount == 0 {
		fmt.Printf("%s\n", warnStyle.Render("⚠ Sonarr filtering removed all episode programs; keeping original list"))
		return programs
	}

	if verbose && filteredCount < originalCount {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📺 Sonarr availability filtered %d episode(s)", originalCount-filteredCount)))
	}

	return filtered
}

type radarrMovieIndex struct {
	byTitleYear map[string]struct{}
	byTitle     map[string]struct{}
}

func buildRadarrMovieIndex(movies []radarr.Movie) radarrMovieIndex {
	index := radarrMovieIndex{
		byTitleYear: make(map[string]struct{}),
		byTitle:     make(map[string]struct{}),
	}
	for _, movie := range movies {
		if !movie.HasFile || movie.Title == "" {
			continue
		}
		titleKey := normalizeTitle(movie.Title)
		index.byTitle[titleKey] = struct{}{}
		index.byTitleYear[movieKey(movie.Title, movie.Year)] = struct{}{}
	}
	return index
}

func filterMoviePrograms(programs []tunarr.Program, index radarrMovieIndex) ([]tunarr.Program, int, int) {
	filtered := make([]tunarr.Program, 0, len(programs))
	originalMovieCount := 0
	filteredMovieCount := 0

	for _, program := range programs {
		if program.Type != "movie" {
			filtered = append(filtered, program)
			continue
		}
		originalMovieCount++

		if isRadarrMovieAvailable(program, index) {
			filtered = append(filtered, program)
			filteredMovieCount++
		}
	}

	return filtered, originalMovieCount, filteredMovieCount
}

func isRadarrMovieAvailable(program tunarr.Program, index radarrMovieIndex) bool {
	if program.Title == "" {
		return false
	}
	if program.Year > 0 {
		if _, ok := index.byTitleYear[movieKey(program.Title, program.Year)]; ok {
			return true
		}
	}
	_, ok := index.byTitle[normalizeTitle(program.Title)]
	return ok
}

func buildSonarrEpisodeIndex(seriesByID map[int]sonarr.Series, episodes []sonarr.Episode) map[string]struct{} {
	index := make(map[string]struct{})
	for _, episode := range episodes {
		if !episode.HasFile {
			continue
		}
		series, ok := seriesByID[episode.SeriesID]
		if !ok || series.Title == "" {
			continue
		}
		key := episodeKey(series.Title, episode.SeasonNumber, episode.EpisodeNumber)
		index[key] = struct{}{}
	}
	return index
}

func filterEpisodePrograms(programs []tunarr.Program, available map[string]struct{}) ([]tunarr.Program, int, int) {
	filtered := make([]tunarr.Program, 0, len(programs))
	originalEpisodeCount := 0
	filteredEpisodeCount := 0

	for _, program := range programs {
		if program.Type != "episode" {
			filtered = append(filtered, program)
			continue
		}
		originalEpisodeCount++

		if isSonarrEpisodeAvailable(program, available) {
			filtered = append(filtered, program)
			filteredEpisodeCount++
		}
	}

	return filtered, originalEpisodeCount, filteredEpisodeCount
}

func isSonarrEpisodeAvailable(program tunarr.Program, available map[string]struct{}) bool {
	if program.ShowTitle == "" || program.Season == 0 || program.Episode == 0 {
		return true
	}
	_, ok := available[episodeKey(program.ShowTitle, program.Season, program.Episode)]
	return ok
}

func movieKey(title string, year int) string {
	return normalizeTitle(title) + "|" + strconv.Itoa(year)
}

func episodeKey(showTitle string, season, episode int) string {
	return normalizeTitle(showTitle) + "|s" + strconv.Itoa(season) + "e" + strconv.Itoa(episode)
}

func normalizeTitle(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}
