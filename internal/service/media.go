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
// covers a flat-shaped fixture or test double directly -- and, for a real
// Tunarr instance, tunarr.Client.SearchPrograms hydrates it from the
// nested "show" object a live "episode" result actually carries
// (hydrateEpisodeShowFields in internal/external/tunarr/client.go, the
// single choke point that also covers GetFillerPrograms and every
// library-scoped search fetchLibraryPrograms below issues). So a program
// with an empty ShowTitle here means Tunarr genuinely has no show data for
// it (or it isn't an episode at all), not a client-side deserialization
// gap -- that gap (see git history around this comment, and this task's
// report, for the "used to be tagged json:-" and later "nested vs. flat"
// history) is now closed.
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
// both movie and episode entries directly), a live Tunarr "episode" result
// carries no rating of its own at all -- only its nested "show" object
// does (see docs/tunarr/openapi.json's Episode/Show schemas, and
// tunarr.Program.ShowTitle's doc comment in models.go). The same
// hydrateEpisodeShowFields choke point MediaShows' doc comment describes
// fills Program.Rating from Show.Rating whenever an episode's own Rating
// came back empty, so episodes now contribute to Ratings here exactly like
// movies do -- this is a read of the same client-side hydration, not a
// second implementation of it.
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
