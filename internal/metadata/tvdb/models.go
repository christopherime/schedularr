package tvdb

// loginRequest is the body of POST /login. PIN is only set for a
// user-supported subscriber key; a project key omits it.
type loginRequest struct {
	APIKey string `json:"apikey"`
	PIN    string `json:"pin,omitempty"`
}

// loginResponse carries the bearer token every other v4 route requires.
type loginResponse struct {
	Status string `json:"status"`
	Data   struct {
		Token string `json:"token"`
	} `json:"data"`
}

// searchResponse is the envelope of GET /search.
type searchResponse struct {
	Status string         `json:"status"`
	Data   []searchResult `json:"data"`
}

// searchResult is one search hit. TVDBID is the bare numeric ID as a
// string; the sibling "id" field is a prefixed form ("series-121361")
// that no other route accepts.
type searchResult struct {
	TVDBID   string   `json:"tvdb_id"`
	Name     string   `json:"name"`
	Year     string   `json:"year"`
	Overview string   `json:"overview"`
	ImageURL string   `json:"image_url"`
	Genres   []string `json:"genres"`
}

// seriesExtendedResponse is the envelope of GET
// /series/{id}/extended.
type seriesExtendedResponse struct {
	Status string         `json:"status"`
	Data   seriesExtended `json:"data"`
}

// seriesExtended is the extended series record: the only v4 route that
// returns genres, certifications and cross-references together.
type seriesExtended struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	Year           string          `json:"year"`
	FirstAired     string          `json:"firstAired"`
	Overview       string          `json:"overview"`
	Image          string          `json:"image"`
	Genres         []genre         `json:"genres"`
	ContentRatings []contentRating `json:"contentRatings"`
	RemoteIDs      []remoteID      `json:"remoteIds"`
}

// genre is TheTVDB's genre record; only Name is used.
type genre struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// contentRating is one country's certification. Country is a lowercased
// three-letter code ("usa"), not ISO 3166-1 alpha-2.
type contentRating struct {
	Name        string `json:"name"`
	Country     string `json:"country"`
	ContentType string `json:"contentType"`
}

// remoteID is a cross-reference to another database. SourceName is a
// display string ("IMDB", "TheMovieDB.com"), not a stable key -- see
// externalIDKey.
type remoteID struct {
	ID         string `json:"id"`
	SourceName string `json:"sourceName"`
}
