package service

import (
	"context"
	"sort"
)

// MediaShow is one distinct TV show observed in Tunarr's synced library:
// Title is the show's grouping key (tunarr.Program.ShowTitle) and
// EpisodeCount is how many Type == "episode" programs in the fetched
// library carry that title.
type MediaShow struct {
	Title        string
	EpisodeCount int
}

// MediaMeta is the distinct set of genre and rating values observed across
// every program (movies and episodes alike) in Tunarr's synced library,
// each sorted ascending.
type MediaMeta struct {
	Genres  []string
	Ratings []string
}

// MediaShows returns every distinct show Runner's fetchPrograms sees,
// sorted by title, with each show's episode count. It calls fetchPrograms
// directly -- the same fetch-then-cache path Run uses to build its
// scheduling candidate pool (fetchLibraryPrograms, falling back to
// fetchAllProgramsViaSearch, cached for contentCacheDuration) -- rather
// than standing up a second cache or a second fetch shape: a MediaShows
// call served from a warm cache costs nothing beyond the grouping below,
// and a call that primes the cache (or finds it already primed by a prior
// Run/MediaMeta call) makes every one of those callers see the same
// library snapshot for the rest of the cache's lifetime.
//
// Only Type == "episode" programs are grouped; a movie has no show to
// belong to. A program whose ShowTitle is empty is skipped rather than
// folded into a bogus "" show. tunarr.Program.ShowTitle is tagged
// `json:"showTitle,omitempty"` (internal/external/tunarr/models.go), which
// covers a flat-shaped fixture or test double directly. Against a real
// Tunarr instance, an episode result never carries a flat ShowTitle (or a
// nested "show" object -- an earlier round of this fix wrongly assumed
// one, see models.go's history) at all; it carries only a ShowID foreign
// key. fetchPrograms below (via fetchSingleLibrary/fetchAllProgramsViaSearch
// in schedule.go) runs every fetch through hydrateShowsAndSeasons, which
// joins that ShowID against the separate Type == "show" entries a live
// search interleaves in the same result stream, live-verified against a
// real Tunarr 1.3.13 instance this session (transcript in this task's
// report). So a program with an empty ShowTitle here means either Tunarr
// genuinely has no show data for it, or its ShowID didn't resolve to any
// show entry Tunarr returned -- not a client-side deserialization gap.
func (r *Runner) MediaShows(ctx context.Context) ([]MediaShow, error) {
	programs, err := r.fetchPrograms(ctx)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, p := range programs {
		if p.Type != "episode" || p.ShowTitle == "" {
			continue
		}
		counts[p.ShowTitle]++
	}

	shows := make([]MediaShow, 0, len(counts))
	for title, count := range counts {
		shows = append(shows, MediaShow{Title: title, EpisodeCount: count})
	}
	sort.Slice(shows, func(i, j int) bool { return shows[i].Title < shows[j].Title })

	return shows, nil
}

// MediaMeta returns the distinct genre and rating values observed across
// every fetched program, each sorted ascending. Like MediaShows, it reuses
// fetchPrograms exactly -- the same fetch+cache path, no separate one --
// so a MediaShows call and a MediaMeta call against a warm cache share one
// underlying library snapshot and issue no Tunarr HTTP requests between
// them.
//
// Unlike genres (tunarr.Program.Genres, tagged "genres" and populated for
// both movie and episode entries directly -- live-verified this session),
// a live Tunarr "episode" result carries no rating of its own at all --
// only its show does, via the ShowID join MediaShows' doc comment
// describes (fetchPrograms -> hydrateShowsAndSeasons ->
// hydrateShowTitleAndRating in schedule.go). So episodes contribute to
// Ratings here exactly like movies do once that join runs -- this reads
// the same join's output, not a second implementation of it. Note this
// aggregate can also pick up a rating directly from a Type == "show"
// entry itself (those pass through fetchPrograms' returned slice
// unfiltered, and this loop reads every program's Rating, not just
// episodes') -- harmless for this method's purpose (it only cares about
// the distinct set of ratings observed), but worth knowing if you're
// trying to prove the join specifically: see
// internal/service/schedule_test.go's TestRunner_hydrateShowTitleAndRating
// for a test that isolates it.
func (r *Runner) MediaMeta(ctx context.Context) (*MediaMeta, error) {
	programs, err := r.fetchPrograms(ctx)
	if err != nil {
		return nil, err
	}

	genres := make(map[string]struct{})
	ratings := make(map[string]struct{})
	for _, p := range programs {
		for _, g := range p.GetGenreNames() {
			genres[g] = struct{}{}
		}
		if p.Rating != "" {
			ratings[p.Rating] = struct{}{}
		}
	}

	return &MediaMeta{
		Genres:  sortedSetKeys(genres),
		Ratings: sortedSetKeys(ratings),
	}, nil
}

// sortedSetKeys returns m's keys as a sorted, non-nil slice (empty when m
// is empty), so MediaMeta's Genres/Ratings always marshal to `[]`, never
// `null`.
func sortedSetKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
