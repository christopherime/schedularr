package tvdb

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/christopherime/schedularr/internal/metadata"
)

const (
	// fakeAPIKey and fakeToken are obviously-fake stand-ins. No test here
	// reaches the real TheTVDB.
	fakeAPIKey = "fake-tvdb-key-0000000000000000"
	fakeToken  = "fake-bearer-token-0000000000000000"
)

// loginBody is a POST /login response.
const loginBody = `{"status":"success","data":{"token":"` + fakeToken + `"}}`

// searchBody is a GET /search response. TheTVDB's search carries genre
// names already, but as a flat string list and without certifications
// or cross-references -- which is why a lookup goes on to the extended
// record.
const searchBody = `{
	"status": "success",
	"data": [
		{
			"objectID": "series-121361",
			"id": "series-121361",
			"tvdb_id": "121361",
			"name": "Game of Thrones",
			"year": "2011",
			"overview": "Seven noble families fight for control of Westeros.",
			"image_url": "https://artworks.test/series/121361/poster.jpg",
			"genres": ["Adventure", "Drama"],
			"primary_type": "series"
		}
	]
}`

// extendedBody is a GET /series/{id}/extended response, carrying a
// TheTVDB-only genre spelling ("Suspense"), a non-US certification
// ahead of the US one, and cross-references whose sourceName is a
// display label rather than a stable key.
const extendedBody = `{
	"status": "success",
	"data": {
		"id": 121361,
		"name": "Game of Thrones",
		"year": "2011",
		"firstAired": "2011-04-17",
		"overview": "Seven noble families fight for control of the mythical land of Westeros.",
		"image": "https://artworks.test/series/121361/poster-extended.jpg",
		"genres": [
			{"id": 2, "name": "Adventure", "slug": "adventure"},
			{"id": 9, "name": "Drama", "slug": "drama"},
			{"id": 53, "name": "Suspense", "slug": "suspense"}
		],
		"contentRatings": [
			{"id": 1, "name": "16", "country": "deu", "contentType": "episodeguide"},
			{"id": 2, "name": "TV-MA", "country": "usa", "contentType": "episodeguide"}
		],
		"remoteIds": [
			{"id": "tt0944947", "type": 2, "sourceName": "IMDB"},
			{"id": "1399", "type": 12, "sourceName": "TheMovieDB.com"},
			{"id": "10.5240/xxxx", "type": 3, "sourceName": "EIDR"}
		]
	}
}`

// newTestClient builds a client pointed at srv.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	client, err := New(Config{APIKey: fakeAPIKey, BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return client
}

// TestClient_LookupShow walks the happy path and pins the wire contract
// on the way: the key travels in the login body (never a header or a
// URL), the minted token then rides as an RFC 6750 bearer header on
// every other call, and the search is restricted to series with the
// year hint attached.
func TestClient_LookupShow(t *testing.T) {
	var loginBodySeen loginRequest
	var searchQuery, searchAuth, extendedAuth string

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST on /login, got %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&loginBodySeen); err != nil {
			t.Fatalf("failed to decode the login body: %v", err)
		}
		writeJSON(t, w, http.StatusOK, loginBody)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		searchQuery = r.URL.RawQuery
		searchAuth = r.Header.Get("Authorization")
		writeJSON(t, w, http.StatusOK, searchBody)
	})
	mux.HandleFunc("/series/121361/extended", func(w http.ResponseWriter, r *http.Request) {
		extendedAuth = r.Header.Get("Authorization")
		writeJSON(t, w, http.StatusOK, extendedBody)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	show, err := newTestClient(t, server).LookupShow(context.Background(), "Game of Thrones", 2011)
	if err != nil {
		t.Fatalf("LookupShow returned error: %v", err)
	}

	if loginBodySeen.APIKey != fakeAPIKey {
		t.Errorf("login apikey = %q, want the configured key", loginBodySeen.APIKey)
	}
	if loginBodySeen.PIN != "" {
		t.Errorf("login pin = %q, want it omitted for a project key", loginBodySeen.PIN)
	}
	if searchAuth != "Bearer "+fakeToken {
		t.Errorf("search Authorization = %q, want the minted bearer token", searchAuth)
	}
	if extendedAuth != "Bearer "+fakeToken {
		t.Errorf("extended Authorization = %q, want the minted bearer token", extendedAuth)
	}
	if got := queryParam(searchQuery, "type"); got != "series" {
		t.Errorf("search type = %q, want %q", got, "series")
	}
	if got := queryParam(searchQuery, "query"); got != "Game of Thrones" {
		t.Errorf("search query = %q, want %q", got, "Game of Thrones")
	}
	if got := queryParam(searchQuery, "year"); got != "2011" {
		t.Errorf("search year = %q, want %q", got, "2011")
	}

	if show.Title != "Game of Thrones" {
		t.Errorf("Title = %q, want %q", show.Title, "Game of Thrones")
	}
	if show.Year != 2011 {
		t.Errorf("Year = %d, want 2011", show.Year)
	}
	wantGenres := []string{"Adventure", "Drama", "Thriller"}
	if !reflect.DeepEqual(show.Genres, wantGenres) {
		t.Errorf("Genres = %v, want %v (Suspense must normalize to Thriller)", show.Genres, wantGenres)
	}
	if show.Rating != "TV-MA" {
		t.Errorf("Rating = %q, want the US certification %q", show.Rating, "TV-MA")
	}
	if show.PosterURL != "https://artworks.test/series/121361/poster-extended.jpg" {
		t.Errorf("PosterURL = %q, want the extended record's image", show.PosterURL)
	}
	wantIDs := map[string]string{"tvdb": "121361", "imdb": "tt0944947", "tmdb": "1399"}
	if !reflect.DeepEqual(show.ExternalIDs, wantIDs) {
		t.Errorf("ExternalIDs = %v, want %v (EIDR has no key and must be dropped)", show.ExternalIDs, wantIDs)
	}
	if !strings.Contains(show.Overview, "mythical land") {
		t.Errorf("Overview = %q, want the extended record's synopsis", show.Overview)
	}
}

// TestClient_LookupShow_ReusesToken pins that a token is minted once and
// reused: TheTVDB rate-limits, and one login per lookup would double
// every enrichment pass's request count.
func TestClient_LookupShow_ReusesToken(t *testing.T) {
	var logins int

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		logins++
		writeJSON(t, w, http.StatusOK, loginBody)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, searchBody)
	})
	mux.HandleFunc("/series/121361/extended", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, extendedBody)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client := newTestClient(t, server)
	for i := range 3 {
		if _, err := client.LookupShow(context.Background(), "Game of Thrones", 0); err != nil {
			t.Fatalf("lookup %d returned error: %v", i, err)
		}
	}

	if logins != 1 {
		t.Errorf("expected exactly 1 login across 3 lookups, got %d", logins)
	}
}

// TestClient_LookupShow_SendsPIN pins that a subscriber key's PIN
// reaches the login body, and only when one was configured.
func TestClient_LookupShow_SendsPIN(t *testing.T) {
	var seen loginRequest

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&seen); err != nil {
			t.Fatalf("failed to decode the login body: %v", err)
		}
		writeJSON(t, w, http.StatusOK, loginBody)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, searchBody)
	})
	mux.HandleFunc("/series/121361/extended", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, extendedBody)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	client, err := New(Config{APIKey: fakeAPIKey, PIN: "fake-pin", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := client.LookupShow(context.Background(), "Game of Thrones", 0); err != nil {
		t.Fatalf("LookupShow returned error: %v", err)
	}
	if seen.PIN != "fake-pin" {
		t.Errorf("login pin = %q, want the configured subscriber pin", seen.PIN)
	}
}

// TestClient_LookupShow_NotFound pins that an empty search result is a
// normal ErrNotFound outcome and that no extended request follows it.
func TestClient_LookupShow_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, loginBody)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"status":"success","data":[]}`)
	})
	mux.HandleFunc("/series/", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no extended request may follow a search that matched nothing")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := newTestClient(t, server).LookupShow(context.Background(), "Nonexistent Show", 0)
	if !errors.Is(err, metadata.ErrNotFound) {
		t.Fatalf("expected an error wrapping ErrNotFound, got %v", err)
	}
}

// TestClient_LookupShow_LoginRejected covers a rejected key: the failure
// is ErrUnauthorized (fatal to a whole pass), never ErrNotFound, and no
// search is attempted without a token.
func TestClient_LookupShow_LoginRejected(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusUnauthorized, `{"status":"failure","message":"Unauthorized"}`)
	})
	mux.HandleFunc("/search", func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("no search may be attempted without a token")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := newTestClient(t, server).LookupShow(context.Background(), "Game of Thrones", 0)
	if !errors.Is(err, metadata.ErrUnauthorized) {
		t.Fatalf("expected an error wrapping ErrUnauthorized, got %v", err)
	}
	if errors.Is(err, metadata.ErrNotFound) {
		t.Error("a rejected key must not read as ErrNotFound")
	}
}

// TestClient_LookupShow_LoginWithoutToken covers a 200 login whose body
// carries no token -- an authentication failure however successful the
// status line looks.
func TestClient_LookupShow_LoginWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, `{"status":"success","data":{}}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).LookupShow(context.Background(), "Game of Thrones", 0)
	if !errors.Is(err, metadata.ErrUnauthorized) {
		t.Fatalf("expected an error wrapping ErrUnauthorized, got %v", err)
	}
}

// TestClient_LookupShow_FallsBackToSearchFields covers an extended
// record TheTVDB returned sparsely populated: the search hit fills the
// genres, poster and synopsis it left blank.
func TestClient_LookupShow_FallsBackToSearchFields(t *testing.T) {
	const sparseExtended = `{
		"status": "success",
		"data": {"id": 121361, "name": "", "genres": [], "contentRatings": [], "remoteIds": []}
	}`

	mux := http.NewServeMux()
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, loginBody)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, searchBody)
	})
	mux.HandleFunc("/series/121361/extended", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, http.StatusOK, sparseExtended)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	show, err := newTestClient(t, server).LookupShow(context.Background(), "Game of Thrones", 0)
	if err != nil {
		t.Fatalf("LookupShow returned error: %v", err)
	}

	if show.Title != "Game of Thrones" {
		t.Errorf("Title = %q, want the search hit's name", show.Title)
	}
	if show.Year != 2011 {
		t.Errorf("Year = %d, want 2011 from the search hit", show.Year)
	}
	wantGenres := []string{"Adventure", "Drama"}
	if !reflect.DeepEqual(show.Genres, wantGenres) {
		t.Errorf("Genres = %v, want %v from the search hit", show.Genres, wantGenres)
	}
	if show.PosterURL != "https://artworks.test/series/121361/poster.jpg" {
		t.Errorf("PosterURL = %q, want the search hit's image_url", show.PosterURL)
	}
	if show.Rating != "" {
		t.Errorf("Rating = %q, want empty when no certification was published", show.Rating)
	}
	wantIDs := map[string]string{"tvdb": "121361"}
	if !reflect.DeepEqual(show.ExternalIDs, wantIDs) {
		t.Errorf("ExternalIDs = %v, want %v", show.ExternalIDs, wantIDs)
	}
}

// TestPickMatch pins how a year hint breaks ties between same-named
// series.
func TestPickMatch(t *testing.T) {
	results := []searchResult{
		{TVDBID: "1", Name: "The Office", Year: "2001"},
		{TVDBID: "2", Name: "The Office", Year: "2005"},
	}

	tests := []struct {
		name   string
		hits   []searchResult
		year   int
		wantID string
	}{
		{name: "no hint takes thetvdb's own order", hits: results, year: 0, wantID: "1"},
		{name: "an exact-year hit wins", hits: results, year: 2005, wantID: "2"},
		{name: "an unmatched year falls back to the first hit", hits: results, year: 1999, wantID: "1"},
		{name: "an empty result set has no match", hits: nil, year: 2005, wantID: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickMatch(tt.hits, tt.year)
			if tt.wantID == "" {
				if got != nil {
					t.Fatalf("expected no match, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected the hit with ID %s, got no match", tt.wantID)
			}
			if got.TVDBID != tt.wantID {
				t.Errorf("picked ID %s, want %s", got.TVDBID, tt.wantID)
			}
		})
	}
}

// TestExternalIDKey pins the substring matching the display source
// names force, and that an unrecognized source is dropped rather than
// guessed at.
func TestExternalIDKey(t *testing.T) {
	tests := map[string]string{
		"IMDB":             "imdb",
		"imdb":             "imdb",
		"TheMovieDB.com":   "tmdb",
		"TMDB":             "tmdb",
		"EIDR":             "",
		"Official Website": "",
		"TMS (Zap2It)":     "",
		"":                 "",
	}

	for source, want := range tests {
		if got := externalIDKey(source); got != want {
			t.Errorf("externalIDKey(%q) = %q, want %q", source, got, want)
		}
	}
}

// TestParseYear pins the two date shapes TheTVDB mixes -- a bare year on
// the series record, a full date on firstAired.
func TestParseYear(t *testing.T) {
	tests := map[string]int{
		"2011":       2011,
		"2011-04-17": 2011,
		"":           0,
		"20":         0,
		"not-a-year": 0,
	}

	for value, want := range tests {
		if got := parseYear(value); got != want {
			t.Errorf("parseYear(%q) = %d, want %d", value, got, want)
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
	if client.Name() != "tvdb" {
		t.Errorf("Name = %q, want %q", client.Name(), "tvdb")
	}
}

// TestClient_LookupShow_RequiresTitle pins that a blank title never
// reaches the network -- not even for a login.
func TestClient_LookupShow_RequiresTitle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("a blank title must not reach the server")
	}))
	defer server.Close()

	if _, err := newTestClient(t, server).LookupShow(context.Background(), "  ", 0); err == nil {
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
