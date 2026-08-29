package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/store"
)

// fakeLibraryTunarr is an httptest-backed Tunarr double reporting one media
// source with one library, purpose-built for MediaShows/MediaMeta's tests:
// unlike this file's sibling fakeTunarr (schedule_test.go), which always
// reports zero media sources and so only ever reaches its programs through
// fetchAllProgramsViaSearch -- the *uncached* fallback fetchPrograms takes
// when fetchLibraryPrograms comes back empty (see fetchPrograms's doc
// comment in schedule.go) -- this fake makes fetchLibraryPrograms itself
// return f.programs, which IS the path Runner's cache actually covers
// (fetchTunarrContent's tryLoadFromCache/saveToCache). That's required for
// this file's cache-reuse assertions to mean anything: against the
// zero-media-source fake, every call would hit /api/programs/search again
// regardless of whether MediaShows/MediaMeta reused Run's cache correctly.
// Every request across all three endpoints is counted so tests can assert
// a second call adds none.
type fakeLibraryTunarr struct {
	programs []tunarr.Program

	mu    sync.Mutex
	count int
}

func newFakeLibraryTunarr(t *testing.T, programs []tunarr.Program) (*httptest.Server, *fakeLibraryTunarr) {
	t.Helper()
	f := &fakeLibraryTunarr{programs: programs}

	inc := func() {
		f.mu.Lock()
		f.count++
		f.mu.Unlock()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/media-sources", func(w http.ResponseWriter, r *http.Request) {
		inc()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.MediaSource{{ID: "src-1", Name: "Plex", Type: "plex"}})
	})
	mux.HandleFunc("/api/media-sources/src-1/libraries", func(w http.ResponseWriter, r *http.Request) {
		inc()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tunarr.Library{{ID: "lib-1", Name: "Library", MediaType: "shows"}})
	})
	mux.HandleFunc("/api/programs/search", func(w http.ResponseWriter, r *http.Request) {
		inc()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tunarr.ProgramSearchResponse{
			Results:    f.programs,
			Page:       1,
			TotalPages: 1,
			TotalHits:  len(f.programs),
		})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server, f
}

// requestCount is the total number of HTTP requests the fake has served
// across /api/media-sources, /api/media-sources/{id}/libraries, and
// /api/programs/search combined.
func (f *fakeLibraryTunarr) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.count
}

// newMediaTestRunner builds a bare Runner (no store-side blocks needed --
// MediaShows/MediaMeta never touch the block store) against the given
// Tunarr base URL.
func newMediaTestRunner(t *testing.T, tunarrURL string) *Runner {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = st.Close() })

	client := tunarr.NewClient(tunarr.Config{URL: tunarrURL})
	return NewRunner(st, client, discardLogger(), time.UTC, 0)
}

// mediaTestPrograms seeds two shows (three "The Office" episodes, one
// "Parks and Recreation" episode) plus one movie, with overlapping and
// distinct genres/ratings, so grouping, dedup, and sort all have something
// real to prove.
func mediaTestPrograms() []tunarr.Program {
	return []tunarr.Program{
		{
			ID: "e1", Type: "episode", Title: "Pilot", ShowTitle: "The Office",
			SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000,
			Rating: "TV-14", Genres: []tunarr.Genre{{Name: "Comedy"}},
		},
		{
			ID: "e2", Type: "episode", Title: "Diversity Day", ShowTitle: "The Office",
			SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_320_000,
			Genres: []tunarr.Genre{{Name: "Comedy"}},
		},
		{
			ID: "e3", Type: "episode", Title: "The Alliance", ShowTitle: "The Office",
			SeasonNumber: 1, EpisodeNumber: 3, Duration: 1_320_000,
		},
		{
			ID: "e4", Type: "episode", Title: "Pilot", ShowTitle: "Parks and Recreation",
			SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000,
			Rating: "TV-PG", Genres: []tunarr.Genre{{Name: "Comedy"}, {Name: "Mockumentary"}},
		},
		{
			ID: "m1", Type: "movie", Title: "The Matrix", Duration: 8_160_000,
			Rating: "R", Genres: []tunarr.Genre{{Name: "Action"}, {Name: "Sci-Fi"}},
		},
		{
			// An episode with no ShowTitle, matching what a real Tunarr
			// SearchPrograms response looks like today (see MediaShows'
			// doc comment) -- must not surface as a bogus "" show.
			ID: "e5", Type: "episode", Title: "Untitled", Duration: 1_320_000,
		},
	}
}

// TestRunner_MediaShows_GroupsDedupsAndSortsByTitle covers grouping
// (episodes collapse into their show), dedup (three "The Office" episodes
// become one entry with episode_count 3), sort (titles ascending), the
// movie/empty-ShowTitle exclusion, and -- the core of this task's
// cache-reuse requirement -- that a second call issues no new Tunarr HTTP
// requests at all, proving MediaShows is served from Run's existing
// Runner-scoped cache rather than fetching fresh every call.
func TestRunner_MediaShows_GroupsDedupsAndSortsByTitle(t *testing.T) {
	server, fake := newFakeLibraryTunarr(t, mediaTestPrograms())
	r := newMediaTestRunner(t, server.URL)

	shows, err := r.MediaShows(context.Background())
	require.NoError(t, err)
	require.Equal(t, []MediaShow{
		{Title: "Parks and Recreation", EpisodeCount: 1},
		{Title: "The Office", EpisodeCount: 3},
	}, shows)

	firstCallRequests := fake.requestCount()
	require.Positive(t, firstCallRequests, "the first call must actually reach Tunarr")

	shows2, err := r.MediaShows(context.Background())
	require.NoError(t, err)
	assert.Equal(t, shows, shows2)
	assert.Equal(t, firstCallRequests, fake.requestCount(),
		"a second MediaShows call must be served from Runner's cache, issuing no new Tunarr HTTP requests")
}

// TestRunner_MediaMeta_DistinctSortedGenresAndRatings covers genre/rating
// dedup+sort across both movie and episode programs, and that the same
// cache MediaShows reuses also serves MediaMeta -- no request at all, not
// just no *new* one, since the cache is already warm from the MediaShows
// call above in this test's own body.
func TestRunner_MediaMeta_DistinctSortedGenresAndRatings(t *testing.T) {
	server, fake := newFakeLibraryTunarr(t, mediaTestPrograms())
	r := newMediaTestRunner(t, server.URL)

	meta, err := r.MediaMeta(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"Action", "Comedy", "Mockumentary", "Sci-Fi"}, meta.Genres)
	assert.Equal(t, []string{"R", "TV-14", "TV-PG"}, meta.Ratings)

	firstCallRequests := fake.requestCount()
	require.Positive(t, firstCallRequests)

	meta2, err := r.MediaMeta(context.Background())
	require.NoError(t, err)
	assert.Equal(t, meta, meta2)
	assert.Equal(t, firstCallRequests, fake.requestCount(),
		"a second MediaMeta call must be served from Runner's cache, issuing no new Tunarr HTTP requests")
}

// TestRunner_MediaShowsAndMediaMeta_ShareOneCache is the direct proof
// behind this task's "no second cache" constraint: MediaShows priming the
// cache must mean the very next MediaMeta call -- a different method,
// never called before on this Runner -- serves entirely from that same
// cache too, not a second one scoped to MediaMeta itself.
func TestRunner_MediaShowsAndMediaMeta_ShareOneCache(t *testing.T) {
	server, fake := newFakeLibraryTunarr(t, mediaTestPrograms())
	r := newMediaTestRunner(t, server.URL)

	_, err := r.MediaShows(context.Background())
	require.NoError(t, err)
	afterShows := fake.requestCount()
	require.Positive(t, afterShows)

	_, err = r.MediaMeta(context.Background())
	require.NoError(t, err)
	assert.Equal(t, afterShows, fake.requestCount(),
		"MediaMeta must reuse the cache MediaShows already primed, not fetch again")

	_, err = r.Run(context.Background(), Options{Days: 1, Apply: false})
	// Run also plans blocks against the store, which this Runner has none
	// of, so an empty result is fine here -- only the Tunarr request count
	// matters for this assertion.
	require.NoError(t, err)
	assert.Equal(t, afterShows, fake.requestCount(),
		"Run must also reuse the same warm cache MediaShows/MediaMeta primed")
}

// TestRunner_MediaShows_EmptyLibrary_ReturnsEmptyNonNilSlice pins the
// "empty library with Tunarr up" contract: no programs means an empty
// slice, not nil (nil would marshal to `null`, not the `[]` the API
// contract promises) and not an error.
func TestRunner_MediaShows_EmptyLibrary_ReturnsEmptyNonNilSlice(t *testing.T) {
	server, _ := newFakeLibraryTunarr(t, nil)
	r := newMediaTestRunner(t, server.URL)

	shows, err := r.MediaShows(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, shows)
	assert.Empty(t, shows)
}

// TestRunner_MediaMeta_EmptyLibrary_ReturnsEmptyNonNilSlices mirrors
// TestRunner_MediaShows_EmptyLibrary_ReturnsEmptyNonNilSlice for MediaMeta.
func TestRunner_MediaMeta_EmptyLibrary_ReturnsEmptyNonNilSlices(t *testing.T) {
	server, _ := newFakeLibraryTunarr(t, nil)
	r := newMediaTestRunner(t, server.URL)

	meta, err := r.MediaMeta(context.Background())
	require.NoError(t, err)
	require.NotNil(t, meta)
	assert.NotNil(t, meta.Genres)
	assert.Empty(t, meta.Genres)
	assert.NotNil(t, meta.Ratings)
	assert.Empty(t, meta.Ratings)
}

// nestedShowMediaTestPrograms mirrors mediaTestPrograms above but with no
// episode setting a flat ShowTitle or Rating, only a nested Show object --
// exercising hydrateEpisodeShowFields's SECONDARY, defensive hydration
// path (see its doc comment in internal/external/tunarr/client.go). This
// is NOT what a real Tunarr /api/programs/search "episode" result looks
// like -- live-verified this session (transcript in this task's report)
// that a live episode never nests a "show" object at all, only carries a
// ShowID foreign key; a prior round of this fix claimed the nested shape
// was live-accurate, which was wrong. See liveShapeMediaTestPrograms below
// for the actual live-shaped fixture. Round-tripped through
// fakeLibraryTunarr's real JSON encode/decode, this exercises
// tunarr.Client.SearchPrograms's hydrateEpisodeShowFields, not just this
// package's own field access.
func nestedShowMediaTestPrograms() []tunarr.Program {
	return []tunarr.Program{
		{
			ID: "e1", Type: "episode", Title: "Pilot",
			SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000,
			Genres: []tunarr.Genre{{Name: "Comedy"}},
			Show: &tunarr.Show{
				UUID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Title: "The Office", Rating: "TV-14",
			},
		},
		{
			ID: "e2", Type: "episode", Title: "Diversity Day",
			SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_320_000,
			Genres: []tunarr.Genre{{Name: "Comedy"}},
			Show: &tunarr.Show{
				UUID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Title: "The Office", Rating: "TV-14",
			},
		},
		{
			ID: "e3", Type: "episode", Title: "Pilot",
			SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000,
			Genres: []tunarr.Genre{{Name: "Comedy"}, {Name: "Mockumentary"}},
			Show: &tunarr.Show{
				UUID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb", Title: "Parks and Recreation", Rating: "TV-PG",
			},
		},
		{
			ID: "m1", Type: "movie", Title: "The Matrix", Duration: 8_160_000,
			Rating: "R", Genres: []tunarr.Genre{{Name: "Action"}, {Name: "Sci-Fi"}},
		},
	}
}

// TestRunner_MediaShows_NestedShowObject_HydratesAndGroups pins
// hydrateEpisodeShowFields's secondary path (see
// nestedShowMediaTestPrograms' doc comment): episodes whose show identity
// arrives nested under Program.Show still group into the correct show.
// See TestRunner_MediaShows_LiveShape_HydratesAndGroups below for the
// actual live-shaped (ShowID FK + interleaved show entry) version.
func TestRunner_MediaShows_NestedShowObject_HydratesAndGroups(t *testing.T) {
	server, _ := newFakeLibraryTunarr(t, nestedShowMediaTestPrograms())
	r := newMediaTestRunner(t, server.URL)

	shows, err := r.MediaShows(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []MediaShow{
		{Title: "Parks and Recreation", EpisodeCount: 1},
		{Title: "The Office", EpisodeCount: 2},
	}, shows)
}

// TestRunner_MediaMeta_NestedShowObject_HydratesRatings mirrors
// TestRunner_MediaShows_NestedShowObject_HydratesAndGroups for MediaMeta.
func TestRunner_MediaMeta_NestedShowObject_HydratesRatings(t *testing.T) {
	server, _ := newFakeLibraryTunarr(t, nestedShowMediaTestPrograms())
	r := newMediaTestRunner(t, server.URL)

	meta, err := r.MediaMeta(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"Action", "Comedy", "Mockumentary", "Sci-Fi"}, meta.Genres)
	assert.Equal(t, []string{"R", "TV-14", "TV-PG"}, meta.Ratings)
}

// liveShapeMediaTestPrograms mirrors mediaTestPrograms above but in the
// ACTUAL live Tunarr wire shape -- live-verified this session (transcript
// in this task's report): episodes carry only ShowID foreign keys (no
// flat ShowTitle/Rating, no nested Show object), and their shows are
// separate Type == "show" entries interleaved in the same result set,
// related only by ID. Round-tripped through fakeLibraryTunarr's real
// JSON encode/decode, this exercises
// service.Runner.hydrateShowTitleAndRating -- the actual production join
// -- not the client's defensive nested-Show fallback.
func liveShapeMediaTestPrograms() []tunarr.Program {
	const (
		officeID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		parksID  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)
	return []tunarr.Program{
		{UUID: officeID, Type: "show", Title: "The Office", Rating: "TV-14"},
		{UUID: parksID, Type: "show", Title: "Parks and Recreation", Rating: "TV-PG"},
		{
			ID: "e1", Type: "episode", Title: "Pilot", ShowID: officeID,
			SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000,
			Genres: []tunarr.Genre{{Name: "Comedy"}},
		},
		{
			ID: "e2", Type: "episode", Title: "Diversity Day", ShowID: officeID,
			SeasonNumber: 1, EpisodeNumber: 2, Duration: 1_320_000,
			Genres: []tunarr.Genre{{Name: "Comedy"}},
		},
		{
			ID: "e3", Type: "episode", Title: "Pilot", ShowID: parksID,
			SeasonNumber: 1, EpisodeNumber: 1, Duration: 1_320_000,
			Genres: []tunarr.Genre{{Name: "Comedy"}, {Name: "Mockumentary"}},
		},
		{
			ID: "m1", Type: "movie", Title: "The Matrix", Duration: 8_160_000,
			Rating: "R", Genres: []tunarr.Genre{{Name: "Action"}, {Name: "Sci-Fi"}},
		},
	}
}

// TestRunner_MediaShows_LiveShape_HydratesAndGroups proves GET
// /media/shows returns non-empty, correctly grouped results against a
// live-shaped Tunarr library (ShowID FK + interleaved show entry, no
// nested object) -- the shape a real Tunarr 1.3.13 instance actually
// sends, not just this package's flat-shaped or nested-Show fixtures.
func TestRunner_MediaShows_LiveShape_HydratesAndGroups(t *testing.T) {
	server, _ := newFakeLibraryTunarr(t, liveShapeMediaTestPrograms())
	r := newMediaTestRunner(t, server.URL)

	shows, err := r.MediaShows(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []MediaShow{
		{Title: "Parks and Recreation", EpisodeCount: 1},
		{Title: "The Office", EpisodeCount: 2},
	}, shows)
}

// TestRunner_MediaMeta_LiveShape_HydratesRatings proves MediaMeta returns
// the correct aggregate Ratings/Genres set against live-shaped data
// end-to-end. Caveat found during this fix's revert-verify pass (see
// service.TestRunner_hydrateShowTitleAndRating for the test that actually
// discriminates the join): because Type == "show" entries pass through
// fetchPrograms' returned slice unfiltered, and MediaMeta iterates every
// program's own Rating field, "TV-14"/"TV-PG" reach Ratings here from the
// show entries THEMSELVES (which always carry their own Rating, join or
// no join) as well as from any episode the join successfully hydrates --
// so, unlike TestRunner_MediaShows_LiveShape_HydratesAndGroups (which
// only ever counts Type == "episode" entries and genuinely fails without
// the join), this specific assertion does not go red if
// hydrateShowTitleAndRating is disabled. Kept as honest end-to-end
// coverage of MediaMeta's actual output, not as a join-specific
// regression pin.
func TestRunner_MediaMeta_LiveShape_HydratesRatings(t *testing.T) {
	server, _ := newFakeLibraryTunarr(t, liveShapeMediaTestPrograms())
	r := newMediaTestRunner(t, server.URL)

	meta, err := r.MediaMeta(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"Action", "Comedy", "Mockumentary", "Sci-Fi"}, meta.Genres)
	assert.Equal(t, []string{"R", "TV-14", "TV-PG"}, meta.Ratings)
}
