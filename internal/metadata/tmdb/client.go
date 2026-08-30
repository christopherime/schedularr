// Package tmdb implements metadata.Provider against The Movie
// Database's v3 REST API.
package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/christopherime/schedularr/internal/cache"
	"github.com/christopherime/schedularr/internal/httpclient"
	"github.com/christopherime/schedularr/internal/metadata"
)

const (
	// DefaultBaseURL is TMDB's v3 API root.
	DefaultBaseURL = "https://api.themoviedb.org/3"

	// DefaultPosterBaseURL is TMDB's image CDN at the w500 size. TMDB
	// returns poster paths ("/abc.jpg"), never absolute URLs, so a size
	// prefix has to be chosen client-side; w500 is the size TMDB's own
	// documentation uses for list art.
	DefaultPosterBaseURL = "https://image.tmdb.org/t/p/w500"

	// providerName is this provider's identifier and its key in
	// ShowMetadata.ExternalIDs.
	providerName = "tmdb"

	// defaultLanguage is the tag TMDB answers in when the caller does
	// not choose one.
	defaultLanguage = "en-US"

	// preferredRatingCountry is the certification body ShowMetadata.Rating
	// prefers, matching the TV-* vocabulary Tunarr libraries carry.
	preferredRatingCountry = "US"

	// genreCacheTTL bounds how long the ID-to-name genre table is reused.
	// TMDB's television genre list changes on the order of years.
	genreCacheTTL = 24 * time.Hour

	// genreCacheKey is the single key held in the genre cache.
	genreCacheKey = "tv-genres"

	// errorBodyLimit caps how much of a TMDB error body is quoted back in
	// an error message.
	errorBodyLimit = 200
)

// Config configures a TMDB client.
type Config struct {
	// APIKey is the TMDB v3 API key. The caller supplies it -- this
	// package never reads the environment.
	APIKey string

	// BaseURL overrides DefaultBaseURL. Tests point it at an httptest
	// server.
	BaseURL string

	// PosterBaseURL overrides DefaultPosterBaseURL, to pick a different
	// image size or a mirror.
	PosterBaseURL string

	// Language is the tag TMDB answers in (an ISO 639-1 code, optionally
	// with a region: "en-US", "fr-FR"). Defaults to defaultLanguage.
	Language string
}

// Client is a TMDB v3 API client.
type Client struct {
	http          *httpclient.Client
	apiKey        string
	language      string
	posterBaseURL string

	// genreMu serializes a cold genre-table fetch so that N concurrent
	// lookups cost one request rather than N.
	genreMu sync.Mutex
	genres  *cache.Cache
}

// Compile-time proof that this client satisfies the provider contract.
var _ metadata.Provider = (*Client)(nil)

// New creates a TMDB client. It performs no network call: the only way
// it fails is an empty API key.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("tmdb: api key is required")
	}

	genres, err := cache.New(genreCacheTTL)
	if err != nil {
		return nil, fmt.Errorf("tmdb: failed to create the genre cache: %w", err)
	}

	return &Client{
		http: httpclient.New(httpclient.DefaultConfig(
			strings.TrimRight(orDefault(cfg.BaseURL, DefaultBaseURL), "/"),
			"", // the v3 API takes its key as a query parameter, not a header
			httpclient.AuthNone,
		)),
		apiKey:        cfg.APIKey,
		language:      orDefault(cfg.Language, defaultLanguage),
		posterBaseURL: strings.TrimRight(orDefault(cfg.PosterBaseURL, DefaultPosterBaseURL), "/"),
		genres:        genres,
	}, nil
}

// Name returns "tmdb".
func (c *Client) Name() string { return providerName }

// LookupShow resolves a show against TMDB in two requests: GET
// /search/tv to find it, then GET /tv/{id} for the genres, certification
// and cross-references a search hit does not carry.
func (c *Client) LookupShow(ctx context.Context, title string, year int) (*metadata.ShowMetadata, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("tmdb: title is required")
	}

	hit, err := c.search(ctx, title, year)
	if err != nil {
		return nil, err
	}

	detail, err := c.series(ctx, hit.ID)
	if err != nil {
		return nil, err
	}

	genres, err := c.genreNames(ctx, hit, detail)
	if err != nil {
		return nil, err
	}

	return c.assemble(hit, detail, genres), nil
}

// search runs GET /search/tv and picks the hit to describe.
func (c *Client) search(ctx context.Context, title string, year int) (*searchResult, error) {
	query := c.query()
	query.Set("query", title)
	query.Set("include_adult", "false")
	if year > 0 {
		query.Set("first_air_date_year", strconv.Itoa(year))
	}

	var resp searchResponse
	if err := c.get(ctx, "/search/tv", query, &resp); err != nil {
		return nil, err
	}

	hit := pickMatch(resp.Results, year)
	if hit == nil {
		return nil, fmt.Errorf("tmdb: no series matching %q: %w", title, metadata.ErrNotFound)
	}
	return hit, nil
}

// series runs GET /tv/{id}, appending the two sub-resources a lookup
// needs so that the whole detail costs one request.
func (c *Client) series(ctx context.Context, id int64) (*seriesDetail, error) {
	query := c.query()
	query.Set("append_to_response", "content_ratings,external_ids")

	var detail seriesDetail
	if err := c.get(ctx, "/tv/"+strconv.FormatInt(id, 10), query, &detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

// genreNames prefers the names GET /tv/{id} returns directly, which is
// the path that fires for a fully populated TMDB entry.
//
// The ID fallback below only runs when that array comes back empty --
// TMDB answers with no genres for sparsely populated series -- and maps
// the search hit's genre_ids through GET /genre/tv/list, fetched at most
// once per genreCacheTTL. It is the reason this client keeps a genre
// table at all: a search hit never carries genre names.
func (c *Client) genreNames(ctx context.Context, hit *searchResult, detail *seriesDetail) ([]string, error) {
	if len(detail.Genres) > 0 {
		names := make([]string, 0, len(detail.Genres))
		for _, g := range detail.Genres {
			names = append(names, g.Name)
		}
		return names, nil
	}

	if len(hit.GenreIDs) == 0 {
		return nil, nil
	}

	byID, err := c.genreMap(ctx)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(hit.GenreIDs))
	for _, id := range hit.GenreIDs {
		if name, ok := byID[id]; ok {
			names = append(names, name)
		}
	}
	return names, nil
}

// genreMap returns TMDB's television genre ID-to-name table, fetching it
// at most once per genreCacheTTL.
func (c *Client) genreMap(ctx context.Context) (map[int64]string, error) {
	c.genreMu.Lock()
	defer c.genreMu.Unlock()

	if cached, found := c.genres.Get(genreCacheKey); found {
		if table, ok := cached.(map[int64]string); ok {
			return table, nil
		}
	}

	var resp genreListResponse
	if err := c.get(ctx, "/genre/tv/list", c.query(), &resp); err != nil {
		return nil, err
	}

	table := make(map[int64]string, len(resp.Genres))
	for _, g := range resp.Genres {
		table[g.ID] = g.Name
	}

	if err := c.genres.Set(genreCacheKey, table); err != nil {
		return nil, fmt.Errorf("tmdb: failed to cache the genre table: %w", err)
	}
	return table, nil
}

// assemble folds a search hit and its detail into the provider-agnostic
// shape. The detail wins field by field, with the search hit as a
// fallback for anything TMDB left blank on the detail record.
func (c *Client) assemble(hit *searchResult, detail *seriesDetail, genres []string) *metadata.ShowMetadata {
	ids := map[string]string{providerName: strconv.FormatInt(detail.ID, 10)}
	if detail.ExternalIDs.IMDbID != "" {
		ids["imdb"] = detail.ExternalIDs.IMDbID
	}
	if tvdbID := detail.ExternalIDs.TVDBID.String(); tvdbID != "" && tvdbID != "0" {
		ids["tvdb"] = tvdbID
	}

	return &metadata.ShowMetadata{
		Title:       orDefault(detail.Name, hit.Name),
		Year:        airYear(orDefault(detail.FirstAirDate, hit.FirstAirDate)),
		Genres:      metadata.NormalizeGenres(genres),
		Rating:      pickContentRating(detail.ContentRatings.Results),
		Overview:    orDefault(detail.Overview, hit.Overview),
		PosterURL:   c.posterURL(orDefault(detail.PosterPath, hit.PosterPath)),
		ExternalIDs: ids,
	}
}

// query returns the parameters every request carries, including the API
// key: the v3 API authenticates by query parameter, not by header.
func (c *Client) query() url.Values {
	q := url.Values{}
	q.Set("api_key", c.apiKey)
	q.Set("language", c.language)
	return q
}

// get performs a GET and translates any failure into an error that is
// safe to log (see apiFailure).
func (c *Client) get(ctx context.Context, endpoint string, query url.Values, out any) error {
	if err := c.http.Get(ctx, endpoint+"?"+query.Encode(), out); err != nil {
		return apiFailure(endpoint, err)
	}
	return nil
}

// posterURL turns TMDB's relative poster path into an absolute URL.
func (c *Client) posterURL(path string) string {
	if path == "" {
		return ""
	}
	return c.posterBaseURL + path
}

// apiFailure maps an httpclient error onto the metadata sentinels and --
// deliberately -- never forwards the underlying error.
//
// httpclient.APIError.Error() prints the full request URL, and TMDB v3
// carries the API key in that URL's query string, so wrapping the
// original error would print the operator's key into the log line of
// every failed lookup. Only the endpoint (which has no query string),
// the status, and the response body (which never echoes the key) are
// carried across.
func apiFailure(endpoint string, err error) error {
	var apiErr *httpclient.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("tmdb: GET %s failed", endpoint)
	}

	switch apiErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("tmdb: GET %s: %w (%s)", endpoint, metadata.ErrUnauthorized, snippet(apiErr.Body))
	case http.StatusNotFound:
		return fmt.Errorf("tmdb: GET %s: %w", endpoint, metadata.ErrNotFound)
	case 0:
		if httpclient.IsDecodeError(err) {
			return fmt.Errorf("tmdb: GET %s: response did not decode", endpoint)
		}
		return fmt.Errorf("tmdb: GET %s: tmdb unreachable", endpoint)
	default:
		return fmt.Errorf("tmdb: GET %s: unexpected status %d (%s)", endpoint, apiErr.StatusCode, snippet(apiErr.Body))
	}
}

// pickMatch chooses which search hit to describe. TMDB's
// first_air_date_year parameter already narrows the response when a year
// hint was given, but it matches TMDB's first_air_date rather than the
// library's notion of a year, so an exact-year hit still wins over
// TMDB's own relevance order. Without a hint, or with no exact match,
// TMDB's first (most popular) result wins.
func pickMatch(results []searchResult, year int) *searchResult {
	if len(results) == 0 {
		return nil
	}
	if year > 0 {
		for i := range results {
			if airYear(results[i].FirstAirDate) == year {
				return &results[i]
			}
		}
	}
	return &results[0]
}

// pickContentRating prefers the US certification, falling back to the
// first non-empty entry so that a show TMDB only rated elsewhere still
// gets one.
func pickContentRating(ratings []contentRating) string {
	var fallback string
	for _, r := range ratings {
		if r.Rating == "" {
			continue
		}
		if strings.EqualFold(r.Country, preferredRatingCountry) {
			return r.Rating
		}
		if fallback == "" {
			fallback = r.Rating
		}
	}
	return fallback
}

// airYear extracts the year from TMDB's YYYY-MM-DD date, returning 0 for
// an empty or malformed value.
func airYear(date string) int {
	if len(date) < 4 {
		return 0
	}
	year, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return year
}

// orDefault returns value, or fallback when value is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// snippet caps an error body so a verbose provider error cannot dominate
// a log line.
func snippet(body string) string {
	body = strings.TrimSpace(body)
	if len(body) > errorBodyLimit {
		return body[:errorBodyLimit] + "..."
	}
	return body
}
