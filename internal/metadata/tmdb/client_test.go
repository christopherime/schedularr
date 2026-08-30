package tmdb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/christopherime/schedularr/internal/metadata"
)

// fakeAPIKey is an obviously-fake stand-in. No test here reaches the
// real TMDB.
const fakeAPIKey = "fake-tmdb-key-0000000000000000"

// searchBody is a /search/tv response shaped like TMDB v3's: a hit
// carries genre_ids, never genre names.
const searchBody = `{
	"page": 1,
	"results": [
		{
			"id": 1396,
			"name": "Breaking Bad",
			"first_air_date": "2008-01-20",
			"overview": "A high school chemistry teacher turns to making meth.",
			"poster_path": "/breaking-bad.jpg",
			"genre_ids": [18, 80]
		}
	],
	"total_pages": 1,
	"total_results": 1
}`

// detailBody is a /tv/{id} response with
// append_to_response=content_ratings,external_ids, carrying a compound
// television genre ("Sci-Fi & Fantasy") so the mapping is exercised
// end to end, a non-US certification ahead of the US one, and a numeric
// tvdb_id.
const detailBody = `{
	"id": 1396,
	"name": "Breaking Bad",
	"first_air_date": "2008-01-20",
	"overview": "A high school chemistry teacher turns to making meth.",
	"poster_path": "/breaking-bad.jpg",
	"genres": [
		{"id": 18, "name": "Drama"},
		{"id": 80, "name": "Crime"},
		{"id": 10765, "name": "Sci-Fi & Fantasy"}
	],
	"content_ratings": {
		"results": [
			{"iso_3166_1": "DE", "rating": "16"},
			{"iso_3166_1": "US", "rating": "TV-MA"}
		]
	},
	"external_ids": {
		"imdb_id": "tt0903747",
		"tvdb_id": 81189
	}
}`

// genreListBody is a /genre/tv/list response holding TMDB's real
// television genre IDs.
const genreListBody = `{
	"genres": [
		{"id": 16, "name": "Animation"},
		{"id": 18, "name": "Drama"},
		{"id": 80, "name": "Crime"},
		{"id": 10759, "name": "Action & Adventure"},
		{"id": 10762, "name": "Kids"}
	]
}`

// newTestClient builds a client pointed at srv with a deterministic
// poster base, so poster assertions do not depend on TMDB's CDN.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	client, err := New(Config{
		APIKey:        fakeAPIKey,
		BaseURL:       srv.URL,
		PosterBaseURL: "https://images.test/w500",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return client
}

// TestClient_LookupShow walks the happy path and pins the wire contract
// on the way: both requests authenticate by api_key query parameter (v3
// has no auth header), the search sends the year hint as
// first_air_date_year, and the detail request appends both
// sub-resources so one call covers genres, certification and
// cross-references.
func TestClient_LookupShow(t *testing.T) {
	var searchQuery, detailQuery string

	mux := http.NewServeMux()
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, r *http.Request) {
		searchQuery = r.URL.RawQuery
		writeJSON(t, w, http.StatusOK, searchBody)
	})
	mux.HandleFunc("/tv/1396", func(w http.ResponseWriter, r *http.Request) {
		detailQuery = r.URL.RawQuery
		writeJSON(t, w, http.StatusOK, detailBody)
	})
	mux.HandleFunc("/genre/tv/list", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("the genre table must not be fetched when the detail record already carries genre names")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server)
	show, err := client.LookupShow(context.Background(), "Breaking Bad", 2008)
	if err != nil {
		t.Fatalf("LookupShow returned error: %v", err)
	}

	if got := queryParam(searchQuery, "api_key"); got != fakeAPIKey {
		t.Errorf("search api_key = %q, want the configured key", got)
	}
	if got := queryParam(searchQuery, "query"); got != "Breaking Bad" {
		t.Errorf("search query = %q, want %q", got, "Breaking Bad")
	}
	if got := queryParam(searchQuery, "first_air_date_year"); got != "2008" {
		t.Errorf("search first_air_date_year = %q, want %q", got, "2008")
	}
	if got := queryParam(detailQuery, "append_to_response"); got != "content_ratings,external_ids" {
		t.Errorf("detail append_to_response = %q, want both sub-resources", got)
	}

	if show.Title != "Breaking Bad" {
		t.Errorf("Title = %q, want %q", show.Title, "Breaking Bad")
	}
	if show.Year != 2008 {
		t.Errorf("Year = %d, want 2008", show.Year)
	}
	wantGenres := []string{"Drama", "Crime", "Science Fiction"}
	if !reflect.DeepEqual(show.Genres, wantGenres) {
		t.Errorf("Genres = %v, want %v (Sci-Fi & Fantasy must normalize)", show.Genres, wantGenres)
	}
	if show.Rating != "TV-MA" {
		t.Errorf("Rating = %q, want the US certification %q", show.Rating, "TV-MA")
	}
	if show.PosterURL != "https://images.test/w500/breaking-bad.jpg" {
		t.Errorf("PosterURL = %q, want the poster base joined to the path", show.PosterURL)
	}
	wantIDs := map[string]string{"tmdb": "1396", "imdb": "tt0903747", "tvdb": "81189"}
	if !reflect.DeepEqual(show.ExternalIDs, wantIDs) {
		t.Errorf("ExternalIDs = %v, want %v", show.ExternalIDs, wantIDs)
	}
	if !strings.HasPrefix(show.Overview, "A high school chemistry teacher") {
		t.Errorf("Overview = %q, want the detail synopsis", show.Overview)
	}
}

// TestClient_LookupShow_NotFound pins that an empty result set is a
// normal ErrNotFound outcome, not a transport failure, and that no
// detail request follows a search that matched nothing.
func TestClient_LookupShow_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"page":1,"results":[],"total_pages":0,"total_results":0}`)
	})
	mux.HandleFunc("/tv/", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no detail request may follow a search that matched nothing")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := newTestClient(t, server).LookupShow(context.Background(), "Nonexistent Show", 0)
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("expected an error wrapping ErrNotFound, got %v", err)
	}
}

// TestClient_LookupShow_Unauthorized covers a rejected key, and doubles
// as the regression test for the reason apiFailure refuses to forward
// the underlying error: TMDB v3 authenticates by query parameter, so
// httpclient.APIError.Error() -- which prints the full request URL --
// would otherwise print the operator's API key into the log line of
// every failed lookup.
func TestClient_LookupShow_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized,
			`{"success":false,"status_code":7,"status_message":"Invalid API key: You must be granted a valid key."}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).LookupShow(context.Background(), "Breaking Bad", 0)
	if !errors.Is(err, metadata.ErrUnauthorized) {
		t.Fatalf("expected an error wrapping ErrUnauthorized, got %v", err)
	}
	if errors.Is(err, metadata.ErrNotFound) {
		t.Error("a rejected key must not read as ErrNotFound -- it is fatal to a whole lookup pass")
	}
	if strings.Contains(err.Error(), fakeAPIKey) {
		t.Fatalf("the error leaked the api key: %v", err)
	}
	if !strings.Contains(err.Error(), "Invalid API key") {
		t.Errorf("expected the provider's message to survive, got %v", err)
	}
}

// TestClient_LookupShow_DetailNotFound covers a search hit whose detail
// record is gone (a TMDB entry deleted between the two calls): a 404 on
// the detail request is an ErrNotFound, not an unexpected status.
func TestClient_LookupShow_DetailNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, searchBody)
	})
	mux.HandleFunc("/tv/1396", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusNotFound, `{"success":false,"status_code":34,"status_message":"The resource you requested could not be found."}`)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := newTestClient(t, server).LookupShow(context.Background(), "Breaking Bad", 0)
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("expected an error wrapping ErrNotFound, got %v", err)
	}
}

// TestClient_LookupShow_GenreIDFallback covers the only path that needs
// the genre table: a detail record whose genres array came back empty,
// leaving the search hit's genre_ids as the sole source of genre names.
// It also pins the caching contract -- two lookups fetch
// /genre/tv/list once.
func TestClient_LookupShow_GenreIDFallback(t *testing.T) {
	const detailWithoutGenres = `{
		"id": 1396,
		"name": "Breaking Bad",
		"first_air_date": "2008-01-20",
		"genres": [],
		"content_ratings": {"results": []},
		"external_ids": {}
	}`

	var genreListCalls int
	mux := http.NewServeMux()
	mux.HandleFunc("/search/tv", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, searchBody)
	})
	mux.HandleFunc("/tv/1396", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, detailWithoutGenres)
	})
	mux.HandleFunc("/genre/tv/list", func(w http.ResponseWriter, _ *http.Request) {
		genreListCalls++
		writeJSON(t, w, http.StatusOK, genreListBody)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server)
	for i := range 2 {
		show, err := client.LookupShow(context.Background(), "Breaking Bad", 0)
		if err != nil {
			t.Fatalf("lookup %d returned error: %v", i, err)
		}
		wantGenres := []string{"Drama", "Crime"}
		if !reflect.DeepEqual(show.Genres, wantGenres) {
			t.Errorf("lookup %d Genres = %v, want %v mapped from genre_ids", i, show.Genres, wantGenres)
		}
		if show.Rating != "" {
			t.Errorf("lookup %d Rating = %q, want empty when TMDB published no certification", i, show.Rating)
		}
		if show.ExternalIDs["imdb"] != "" {
			t.Errorf("lookup %d leaked an empty imdb cross-reference: %v", i, show.ExternalIDs)
		}
	}

	if genreListCalls != 1 {
		t.Errorf("expected the genre table to be fetched once and cached, got %d fetches", genreListCalls)
	}
}

// TestPickMatch pins how a year hint breaks ties. TMDB orders search
// results by popularity, so the first hit is not always the one the
// library means.
func TestPickMatch(t *testing.T) {
	results := []searchResult{
		{ID: 1, Name: "The Office", FirstAirDate: "2001-07-09"},
		{ID: 2, Name: "The Office", FirstAirDate: "2005-03-24"},
	}

	tests := []struct {
		name   string
		hits   []searchResult
		year   int
		wantID int64
	}{
		{name: "no hint takes tmdb's own order", hits: results, year: 0, wantID: 1},
		{name: "an exact-year hit wins over popularity", hits: results, year: 2005, wantID: 2},
		{name: "an unmatched year falls back to the first hit", hits: results, year: 1999, wantID: 1},
		{name: "an empty result set has no match", hits: nil, year: 2005, wantID: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickMatch(tt.hits, tt.year)
			if tt.wantID == 0 {
				if got != nil {
					t.Fatalf("expected no match, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected the hit with ID %d, got no match", tt.wantID)
			}
			if got.ID != tt.wantID {
				t.Errorf("picked ID %d, want %d", got.ID, tt.wantID)
			}
		})
	}
}

// TestPickContentRating pins the US preference and its fallback.
func TestPickContentRating(t *testing.T) {
	tests := []struct {
		name    string
		ratings []contentRating
		want    string
	}{
		{
			name:    "the us certification wins wherever it sits",
			ratings: []contentRating{{Country: "DE", Rating: "16"}, {Country: "US", Rating: "TV-MA"}},
			want:    "TV-MA",
		},
		{
			name:    "country matching ignores case",
			ratings: []contentRating{{Country: "us", Rating: "TV-14"}},
			want:    "TV-14",
		},
		{
			name:    "without a us entry the first non-empty one is used",
			ratings: []contentRating{{Country: "FR", Rating: ""}, {Country: "GB", Rating: "15"}},
			want:    "15",
		},
		{name: "no ratings yields empty", ratings: nil, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickContentRating(tt.ratings); got != tt.want {
				t.Errorf("pickContentRating = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestAirYear pins the date parsing, including the malformed values
// TMDB sends for unaired entries.
func TestAirYear(t *testing.T) {
	tests := map[string]int{
		"2008-01-20": 2008,
		"1999":       1999,
		"":           0,
		"20":         0,
		"not-a-date": 0,
	}

	for date, want := range tests {
		if got := airYear(date); got != want {
			t.Errorf("airYear(%q) = %d, want %d", date, got, want)
		}
	}
}

// TestNew_RequiresAPIKey pins that the key is a constructor
// responsibility: this package never falls back to reading it from the
// environment.
func TestNew_RequiresAPIKey(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Error("expected New to reject an empty api key, got nil")
	}
	client, err := New(Config{APIKey: fakeAPIKey})
	if err != nil {
		t.Fatalf("New returned error for a valid config: %v", err)
	}
	if client.Name() != "tmdb" {
		t.Errorf("Name = %q, want %q", client.Name(), "tmdb")
	}
}

// TestClient_LookupShow_RequiresTitle pins that a blank title never
// reaches the network.
func TestClient_LookupShow_RequiresTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("a blank title must not reach the server")
	}))
	defer server.Close()

	if _, err := newTestClient(t, server).LookupShow(context.Background(), "   ", 0); err == nil {
		t.Error("expected an error for a blank title, got nil")
	}
}

// writeJSON writes a fixture body with the status the fake provider is
// simulating.
func writeJSON(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("failed to write the fixture response: %v", err)
	}
}

// queryParam reads one parameter out of a captured raw query string.
func queryParam(rawQuery, key string) string {
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return ""
	}
	return values.Get(key)
}
