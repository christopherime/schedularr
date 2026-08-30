// Package tvdb implements metadata.Provider against TheTVDB's v4 REST
// API.
package tvdb

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

	"github.com/christopherime/schedularr/internal/httpclient"
	"github.com/christopherime/schedularr/internal/metadata"
)

const (
	// DefaultBaseURL is TheTVDB's v4 API root.
	DefaultBaseURL = "https://api4.thetvdb.com/v4"

	// providerName is this provider's identifier and its key in
	// ShowMetadata.ExternalIDs.
	providerName = "tvdb"

	// tokenTTL is how long a minted bearer token is reused before the
	// client logs in again. TheTVDB issues month-long tokens; a day is a
	// deliberately conservative reuse window, chosen so that a token can
	// never be close to expiry in flight and this client never needs a
	// re-authenticate-and-retry path.
	tokenTTL = 24 * time.Hour

	// preferredRatingCountry is the certification body
	// ShowMetadata.Rating prefers. TheTVDB spells countries as
	// lowercased three-letter codes.
	preferredRatingCountry = "usa"

	// errorBodyLimit caps how much of an error body is quoted back in an
	// error message.
	errorBodyLimit = 200
)

// Config configures a TheTVDB client.
type Config struct {
	// APIKey is the TheTVDB v4 API key. The caller supplies it -- this
	// package never reads the environment.
	APIKey string

	// PIN is the subscriber PIN a user-supported key requires. Leave it
	// empty for a project API key.
	PIN string

	// BaseURL overrides DefaultBaseURL. Tests point it at an httptest
	// server.
	BaseURL string
}

// Client is a TheTVDB v4 API client. It logs in lazily on the first
// lookup and reuses the resulting bearer token for tokenTTL.
type Client struct {
	cfg     Config
	baseURL string

	// login is the unauthenticated client, used only for POST /login.
	login *httpclient.Client

	// mu guards authed and expiry, and is deliberately held across the
	// login request so that N concurrent lookups cost one login.
	mu     sync.Mutex
	authed *httpclient.Client
	expiry time.Time
}

// Compile-time proof that this client satisfies the provider contract.
var _ metadata.Provider = (*Client)(nil)

// New creates a TheTVDB client. It performs no network call: the only
// way it fails is an empty API key.
func New(cfg Config) (*Client, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("tvdb: api key is required")
	}

	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	return &Client{
		cfg:     cfg,
		baseURL: baseURL,
		login:   httpclient.New(httpclient.DefaultConfig(baseURL, "", httpclient.AuthNone)),
	}, nil
}

// Name returns "tvdb".
func (c *Client) Name() string { return providerName }

// LookupShow resolves a show against TheTVDB in two authenticated
// requests: GET /search to find it, then GET /series/{id}/extended,
// which is the only v4 route returning genres, certifications and
// cross-references together.
func (c *Client) LookupShow(ctx context.Context, title string, year int) (*metadata.ShowMetadata, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, errors.New("tvdb: title is required")
	}

	authed, err := c.authorized(ctx)
	if err != nil {
		return nil, err
	}

	hit, err := c.search(ctx, authed, title, year)
	if err != nil {
		return nil, err
	}

	detail, err := c.series(ctx, authed, hit.TVDBID)
	if err != nil {
		return nil, err
	}

	return assemble(hit, detail), nil
}

// authorized returns a client carrying a valid bearer token, logging in
// when the cached one is missing or older than tokenTTL.
func (c *Client) authorized(ctx context.Context) (*httpclient.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.authed != nil && time.Now().Before(c.expiry) {
		return c.authed, nil
	}

	var resp loginResponse
	body := loginRequest{APIKey: c.cfg.APIKey, PIN: c.cfg.PIN}
	if err := c.login.Post(ctx, "/login", body, &resp); err != nil {
		return nil, apiFailure("POST /login", err)
	}
	if resp.Data.Token == "" {
		return nil, fmt.Errorf("tvdb: login returned no token: %w", metadata.ErrUnauthorized)
	}

	c.authed = httpclient.New(httpclient.DefaultConfig(c.baseURL, resp.Data.Token, httpclient.AuthBearer))
	c.expiry = time.Now().Add(tokenTTL)
	return c.authed, nil
}

// search runs GET /search restricted to series and picks the hit to
// describe.
func (c *Client) search(ctx context.Context, authed *httpclient.Client, title string, year int) (*searchResult, error) {
	query := url.Values{}
	query.Set("query", title)
	query.Set("type", "series")
	if year > 0 {
		query.Set("year", strconv.Itoa(year))
	}

	var resp searchResponse
	endpoint := "/search"
	if err := authed.Get(ctx, endpoint+"?"+query.Encode(), &resp); err != nil {
		return nil, apiFailure("GET "+endpoint, err)
	}

	hit := pickMatch(resp.Data, year)
	if hit == nil {
		return nil, fmt.Errorf("tvdb: no series matching %q: %w", title, metadata.ErrNotFound)
	}
	return hit, nil
}

// series runs GET /series/{id}/extended.
func (c *Client) series(ctx context.Context, authed *httpclient.Client, id string) (*seriesExtended, error) {
	if id == "" {
		return nil, fmt.Errorf("tvdb: search hit carried no tvdb_id: %w", metadata.ErrNotFound)
	}

	endpoint := "/series/" + url.PathEscape(id) + "/extended"
	var resp seriesExtendedResponse
	if err := authed.Get(ctx, endpoint, &resp); err != nil {
		return nil, apiFailure("GET "+endpoint, err)
	}
	return &resp.Data, nil
}

// assemble folds a search hit and its extended record into the
// provider-agnostic shape. The extended record wins field by field, with
// the search hit as a fallback for anything it left blank.
func assemble(hit *searchResult, detail *seriesExtended) *metadata.ShowMetadata {
	names := make([]string, 0, len(detail.Genres))
	for _, g := range detail.Genres {
		names = append(names, g.Name)
	}
	if len(names) == 0 {
		names = hit.Genres
	}

	ids := map[string]string{providerName: hit.TVDBID}
	for _, remote := range detail.RemoteIDs {
		key := externalIDKey(remote.SourceName)
		if key == "" || remote.ID == "" {
			continue
		}
		if _, taken := ids[key]; !taken {
			ids[key] = remote.ID
		}
	}

	return &metadata.ShowMetadata{
		Title:       orDefault(detail.Name, hit.Name),
		Year:        firstAiredYear(detail, hit),
		Genres:      metadata.NormalizeGenres(names),
		Rating:      pickContentRating(detail.ContentRatings),
		Overview:    orDefault(detail.Overview, hit.Overview),
		PosterURL:   orDefault(detail.Image, hit.ImageURL),
		ExternalIDs: ids,
	}
}

// apiFailure maps an httpclient error onto the metadata sentinels.
//
// Unlike the TMDB client's equivalent, this one forwards the underlying
// error: TheTVDB takes its key in a POST body and then authenticates by
// bearer header, so no request URL here carries a secret that wrapping
// could leak into a log line.
func apiFailure(what string, err error) error {
	var apiErr *httpclient.APIError
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("tvdb: %s failed: %w", what, err)
	}

	switch apiErr.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("tvdb: %s: %w (%s)", what, metadata.ErrUnauthorized, snippet(apiErr.Body))
	case http.StatusNotFound:
		return fmt.Errorf("tvdb: %s: %w", what, metadata.ErrNotFound)
	default:
		return fmt.Errorf("tvdb: %s failed: %w", what, err)
	}
}

// pickMatch chooses which search hit to describe. TheTVDB's year
// parameter already narrows the response when a hint was given, but an
// exact-year hit still wins over TheTVDB's own relevance order. Without
// a hint, or with no exact match, the first result wins.
func pickMatch(results []searchResult, year int) *searchResult {
	if len(results) == 0 {
		return nil
	}
	if year > 0 {
		wanted := strconv.Itoa(year)
		for i := range results {
			if results[i].Year == wanted {
				return &results[i]
			}
		}
	}
	return &results[0]
}

// pickContentRating prefers the US certification, falling back to the
// first non-empty entry so that a show TheTVDB only rated elsewhere
// still gets one.
func pickContentRating(ratings []contentRating) string {
	var fallback string
	for _, r := range ratings {
		if r.Name == "" {
			continue
		}
		if strings.EqualFold(r.Country, preferredRatingCountry) {
			return r.Name
		}
		if fallback == "" {
			fallback = r.Name
		}
	}
	return fallback
}

// externalIDKey maps TheTVDB's display source name onto the key
// ShowMetadata.ExternalIDs uses. The source name is a label rather than
// a stable identifier ("IMDB", "TheMovieDB.com"), so this matches on a
// substring; anything unrecognized ("EIDR", "Official Website") is
// dropped by returning an empty key.
func externalIDKey(sourceName string) string {
	name := strings.ToLower(sourceName)
	switch {
	case strings.Contains(name, "imdb"):
		return "imdb"
	case strings.Contains(name, "themoviedb"), strings.Contains(name, "tmdb"):
		return "tmdb"
	default:
		return ""
	}
}

// firstAiredYear reads the year from the extended record's "year"
// field, its firstAired date, or the search hit's year, in that order.
func firstAiredYear(detail *seriesExtended, hit *searchResult) int {
	for _, candidate := range []string{detail.Year, detail.FirstAired, hit.Year} {
		if year := parseYear(candidate); year > 0 {
			return year
		}
	}
	return 0
}

// parseYear reads a leading four-digit year from "2011" or
// "2011-04-17", returning 0 for anything else.
func parseYear(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, err := strconv.Atoi(value[:4])
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
