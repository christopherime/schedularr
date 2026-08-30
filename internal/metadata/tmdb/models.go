package tmdb

import "encoding/json"

// searchResponse is the envelope of GET /search/tv.
type searchResponse struct {
	Page    int            `json:"page"`
	Results []searchResult `json:"results"`
}

// searchResult is one series hit. A search hit carries genre IDs only --
// the names live behind GET /genre/tv/list (see Client.genreMap).
type searchResult struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	FirstAirDate string  `json:"first_air_date"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	GenreIDs     []int64 `json:"genre_ids"`
}

// seriesDetail is GET /tv/{id} with
// append_to_response=content_ratings,external_ids -- one request for the
// three things a lookup needs that a search hit does not carry.
type seriesDetail struct {
	ID             int64          `json:"id"`
	Name           string         `json:"name"`
	FirstAirDate   string         `json:"first_air_date"`
	Overview       string         `json:"overview"`
	PosterPath     string         `json:"poster_path"`
	Genres         []genre        `json:"genres"`
	ContentRatings contentRatings `json:"content_ratings"`
	ExternalIDs    externalIDs    `json:"external_ids"`
}

// genre is TMDB's ID/name pair, used by both /tv/{id} and
// /genre/tv/list.
type genre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// contentRatings is the append_to_response envelope for per-country
// certifications.
type contentRatings struct {
	Results []contentRating `json:"results"`
}

// contentRating is one country's certification for a series.
type contentRating struct {
	Country string `json:"iso_3166_1"`
	Rating  string `json:"rating"`
}

// externalIDs is the append_to_response envelope for cross-references.
// TVDBID is a json.Number because TMDB sends it as a bare integer (or
// null), while every ID this package publishes is a string.
type externalIDs struct {
	IMDbID string      `json:"imdb_id"`
	TVDBID json.Number `json:"tvdb_id"`
}

// genreListResponse is GET /genre/tv/list.
type genreListResponse struct {
	Genres []genre `json:"genres"`
}
