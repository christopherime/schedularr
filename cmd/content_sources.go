package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/geekxflood/schedularr/internal/cache"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/external/radarr"
	"github.com/geekxflood/schedularr/internal/external/sonarr"
	"github.com/geekxflood/schedularr/internal/external/tunarr"
)

const (
	tunarrCacheKey = "tunarr_programs.json"
	radarrCacheKey = "radarr_movies.json"
	sonarrCacheKey = "sonarr_episodes.json" // Includes series and episodes
)

func fetchAllContent(cfg *config.Config, tunarrClient *tunarr.Client) ([]tunarr.Program, error) {
	fmt.Println(infoStyle.Render("📡 Fetching content..."))

	cacheDuration := cfg.GetCacheDuration()
	contentCache, err := cache.New(cacheDuration)
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not initialize content cache: %v. Proceeding without cache.", err)))
		contentCache = nil // Disable caching if initialization failed
	} else {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🗄️  Using in-memory cache (duration %s)", cacheDuration)))
	}

	programs := fetchTunarrContent(tunarrClient, contentCache)

	if len(programs) == 0 {
		fmt.Println(warnStyle.Render("⚠ No content available from libraries - using fallback SearchPrograms()"))
		var err error
		programs, err = fetchAllProgramsViaSearch(tunarrClient)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch programs: %w", err)
		}
	}

	if cfg.Radarr.URL != "" {
		programs = applyRadarrAvailability(cfg, programs, contentCache)
	}

	if cfg.Sonarr.URL != "" {
		programs = applySonarrAvailability(cfg, programs, contentCache)
	}

	return programs, nil
}

func fetchTunarrContent(client *tunarr.Client, contentCache *cache.Cache) []tunarr.Program {
	var allPrograms []tunarr.Program

	if tryLoadFromCache(contentCache, tunarrCacheKey, &allPrograms) {
		return allPrograms
	}

	allPrograms = fetchLibraryPrograms(client)
	saveTunarrCache(contentCache, allPrograms)

	return allPrograms
}

func tryLoadFromCache(contentCache *cache.Cache, key string, target interface{}) bool {
	if contentCache == nil {
		return false
	}

	found, err := contentCache.Get(key, target)
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Error reading Tunarr programs from cache: %v. Refetching.", err)))
		return false
	}

	if found && verbose {
		fmt.Printf("%s\n", infoStyle.Render("📖 Loaded Tunarr programs from cache"))
	}

	return found
}

func fetchLibraryPrograms(client *tunarr.Client) []tunarr.Program {
	fmt.Println(infoStyle.Render("📡 Fetching content from Tunarr..."))

	// First, get all media sources
	sources, err := client.GetMediaSources(context.Background())
	if err != nil {
		if verbose {
			fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch media sources: %v", err)))
		}
		return nil
	}

	if verbose {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📡 Found %d media source(s)", len(sources))))
	}

	// Collect libraries from all media sources
	var allLibraries []tunarr.Library
	for _, source := range sources {
		libraries, err := client.GetLibraries(context.Background(), source.ID)
		if err != nil {
			if verbose {
				fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch libraries for %s: %v", source.Name, err)))
			}
			continue
		}
		allLibraries = append(allLibraries, libraries...)
	}

	if verbose {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📚 Found %d librar(y/ies)", len(allLibraries))))
	}

	var allPrograms []tunarr.Program
	for _, lib := range allLibraries {
		programs := fetchSingleLibrary(client, lib)
		allPrograms = append(allPrograms, programs...)
	}

	return allPrograms
}

func fetchSingleLibrary(client *tunarr.Client, lib tunarr.Library) []tunarr.Program {
	if verbose {
		fmt.Printf("  - %s (%s)\n", lib.Name, lib.Type)
	}

	// Use SearchPrograms with library ID to get programs from this library
	var allPrograms []tunarr.Program
	page := 1
	limit := 100

	for {
		req := tunarr.ProgramSearchRequest{
			Query:     &tunarr.ProgramSearchQuery{}, // API requires query object
			LibraryID: lib.ID,
			Page:      page,
			Limit:     limit,
		}

		resp, err := client.SearchPrograms(context.Background(), req)
		if err != nil {
			if verbose {
				fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("    ⚠ Could not fetch programs from %s: %v", lib.Name, err)))
			}
			return nil
		}

		allPrograms = append(allPrograms, resp.Results...)

		// Check if we've fetched all programs
		if len(resp.Results) < limit || len(allPrograms) >= resp.Total {
			break
		}
		page++
	}

	if verbose {
		fmt.Printf("    ✓ %d programs\n", len(allPrograms))
	}

	return allPrograms
}

func fetchAllProgramsViaSearch(client *tunarr.Client) ([]tunarr.Program, error) {
	var allPrograms []tunarr.Program
	page := 1
	limit := 100

	for {
		req := tunarr.ProgramSearchRequest{
			Query: &tunarr.ProgramSearchQuery{}, // API requires query object
			Page:  page,
			Limit: limit,
		}

		resp, err := client.SearchPrograms(context.Background(), req)
		if err != nil {
			return nil, err
		}

		allPrograms = append(allPrograms, resp.Results...)

		// Check if we've fetched all programs
		if len(resp.Results) < limit || len(allPrograms) >= resp.Total {
			break
		}
		page++
	}

	return allPrograms, nil
}

func saveTunarrCache(contentCache *cache.Cache, allPrograms []tunarr.Program) {
	if contentCache == nil {
		return
	}

	if err := contentCache.Set(tunarrCacheKey, allPrograms); err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Error writing Tunarr programs to cache: %v", err)))
	}
}

func applyRadarrAvailability(cfg *config.Config, programs []tunarr.Program, contentCache *cache.Cache) []tunarr.Program {
	var movies []radarr.Movie
	var err error
	radarrClient := radarr.NewClient(cfg.Radarr)

	if contentCache != nil {
		var found bool
		found, err = contentCache.Get(radarrCacheKey, &movies)
		if err != nil {
			fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Error reading Radarr movies from cache: %v. Refetching.", err)))
		} else if found {
			if verbose {
				fmt.Printf("%s\n", infoStyle.Render("📖 Loaded Radarr movies from cache"))
			}
			goto ProcessRadarr
		}
	}

	fmt.Println(infoStyle.Render("🎬 Fetching movies from Radarr..."))
	movies, err = radarrClient.GetMovies(context.Background())
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch Radarr movies: %v", err)))
		return programs
	}

	if contentCache != nil {
		if err := contentCache.Set(radarrCacheKey, movies); err != nil {
			fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Error writing Radarr movies to cache: %v", err)))
		}
	}

ProcessRadarr:
	availableTitles := buildRadarrMovieIndex(movies, cfg.Radarr.ExcludeMissingFile)
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

func applySonarrAvailability(cfg *config.Config, programs []tunarr.Program, contentCache *cache.Cache) []tunarr.Program {
	sonarrClient := sonarr.NewClient(cfg.Sonarr)
	series, episodes := loadSonarrData(sonarrClient, contentCache)

	availableEpisodes := buildSonarrEpisodeIndex(buildSeriesMap(series), episodes, cfg.Sonarr.ExcludeMissingFile)
	if len(availableEpisodes) == 0 {
		return programs
	}

	return applyEpisodeFilter(programs, availableEpisodes)
}

type sonarrData struct {
	Series   []sonarr.Series
	Episodes []sonarr.Episode
}

func loadSonarrData(client *sonarr.Client, contentCache *cache.Cache) ([]sonarr.Series, []sonarr.Episode) {
	if contentCache != nil {
		if series, episodes, ok := tryLoadSonarrCache(contentCache); ok {
			return series, episodes
		}
	}

	series, episodes := fetchSonarrData(client)
	saveSonarrCache(contentCache, series, episodes)

	return series, episodes
}

func tryLoadSonarrCache(contentCache *cache.Cache) ([]sonarr.Series, []sonarr.Episode, bool) {
	var cachedData sonarrData
	found, err := contentCache.Get(sonarrCacheKey, &cachedData)
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Error reading Sonarr data from cache: %v. Refetching.", err)))
		return nil, nil, false
	}

	if found && verbose {
		fmt.Printf("%s\n", infoStyle.Render("📖 Loaded Sonarr data from cache"))
	}

	return cachedData.Series, cachedData.Episodes, found
}

func fetchSonarrData(client *sonarr.Client) ([]sonarr.Series, []sonarr.Episode) {
	fmt.Println(infoStyle.Render("📺 Fetching series and episodes from Sonarr..."))
	series, err := client.GetSeries(context.Background())
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch Sonarr series: %v", err)))
		return nil, nil
	}

	var allEpisodes []sonarr.Episode
	for _, s := range series {
		episodes := fetchSeriesEpisodes(client, s)
		allEpisodes = append(allEpisodes, episodes...)
	}

	return series, allEpisodes
}

func fetchSeriesEpisodes(client *sonarr.Client, s sonarr.Series) []sonarr.Episode {
	episodes, err := client.GetEpisodes(context.Background(), s.ID)
	if err != nil {
		if verbose {
			fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("    ⚠ Could not fetch Sonarr episodes for %s: %v", s.Title, err)))
		}
		return nil
	}
	return episodes
}

func saveSonarrCache(contentCache *cache.Cache, series []sonarr.Series, episodes []sonarr.Episode) {
	if contentCache == nil {
		return
	}

	cachedData := sonarrData{
		Series:   series,
		Episodes: episodes,
	}
	if err := contentCache.Set(sonarrCacheKey, cachedData); err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Error writing Sonarr data to cache: %v", err)))
	}
}

func buildSeriesMap(series []sonarr.Series) map[int]sonarr.Series {
	seriesByID := make(map[int]sonarr.Series, len(series))
	for _, s := range series {
		seriesByID[s.ID] = s
	}
	return seriesByID
}

func applyEpisodeFilter(programs []tunarr.Program, availableEpisodes map[string]struct{}) []tunarr.Program {
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

func buildRadarrMovieIndex(movies []radarr.Movie, excludeMissing bool) radarrMovieIndex {
	index := radarrMovieIndex{
		byTitleYear: make(map[string]struct{}),
		byTitle:     make(map[string]struct{}),
	}
	for _, movie := range movies {
		if (excludeMissing && !movie.HasFile) || movie.Title == "" {
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
	if program.GetYear() > 0 {
		if _, ok := index.byTitleYear[movieKey(program.Title, program.GetYear())]; ok {
			return true
		}
	}
	_, ok := index.byTitle[normalizeTitle(program.Title)]
	return ok
}

func buildSonarrEpisodeIndex(seriesByID map[int]sonarr.Series, episodes []sonarr.Episode, excludeMissing bool) map[string]struct{} {
	index := make(map[string]struct{})
	for _, episode := range episodes {
		if excludeMissing && !episode.HasFile {
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
	if program.ShowTitle == "" || program.SeasonNumber == 0 || program.EpisodeNumber == 0 {
		return true
	}
	_, ok := available[episodeKey(program.ShowTitle, program.SeasonNumber, program.EpisodeNumber)]
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
