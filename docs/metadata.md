# Metadata Engine

Tunarr's library genres come from whatever scraper built the source library, and they disagree: TMDB-backed sources publish television compounds like `Sci-Fi & Fantasy`, TheTVDB-backed ones publish `Science Fiction` and `Suspense` separately, and a block's `filter.genres` has to match one of them exactly to select anything. `internal/metadata` closes that gap by looking a show up at an external provider and folding whatever labels come back into a single canonical vocabulary.

!!! note "Scaffolding, not yet wired"

    This page documents the v0.4.0 provider clients and the normalization vocabulary as they exist today. Nothing calls them yet — the scheduling engine's `filter.genres` still matches Tunarr's raw library genres. Configuration keys, key sourcing, and the enrichment pass that stores results are later v0.4.0 slices; see the [Roadmap](roadmap.md).

## The package

`internal/metadata` is provider-agnostic. It defines one result shape and one interface, and the provider subpackages implement it:

```go
type Provider interface {
    Name() string
    LookupShow(ctx context.Context, title string, year int) (*ShowMetadata, error)
}
```

A `ShowMetadata` carries the title, first-air year, normalized genres, content rating, synopsis, an absolute poster URL, and an `ExternalIDs` map keyed by source name (`tmdb`, `tvdb`, `imdb`) — always including the answering provider's own `Name()`, plus any cross-references that provider publishes. Cross-references are what make two providers' answers about one show joinable later.

Two sentinels separate the outcomes a caller has to treat differently:

| Sentinel | Meaning | What a caller does |
| --- | --- | --- |
| `ErrNotFound` | The provider has no match for this title | Skip the show, continue the pass |
| `ErrUnauthorized` | The provider rejected the API key | Abort the pass — every later call fails identically |

Without the second one, a wrong key would read as "none of your 400 shows exist".

## Providers

Both clients are built on `internal/httpclient`, so both inherit its retry behavior: three attempts with exponential backoff on 429, 5xx, and transport failures, which is also how they respect provider rate limits. Neither client reads the environment. The API key is a constructor parameter, so the caller decides where it comes from.

| | `internal/metadata/tmdb` | `internal/metadata/tvdb` |
| --- | --- | --- |
| API | The Movie Database v3, `api.themoviedb.org/3` | TheTVDB v4, `api4.thetvdb.com/v4` |
| Auth | `api_key` query parameter | `POST /login` with the key, then a bearer token |
| Lookup | `GET /search/tv`, then `GET /tv/{id}` | `GET /search?type=series`, then `GET /series/{id}/extended` |
| Genres | From `/tv/{id}`; falls back to mapping the search hit's `genre_ids` through `GET /genre/tv/list` | From the extended record; falls back to the search hit's flat genre list |
| Rating | `content_ratings`, preferring the `US` certification | `contentRatings`, preferring the `usa` certification |
| Cross-refs | `external_ids` (`imdb_id`, `tvdb_id`) | `remoteIds`, matched by source name |

Both take a year hint. A non-zero year narrows the provider-side search and then breaks ties locally: an exact-year hit wins over the provider's own relevance order, because both providers order by popularity and the popular hit is not always the one a library means (there are two shows called *The Office*).

IMDb is deliberately absent. It has no official free API.

### TMDB: two requests, and a genre table

A TMDB search hit carries `genre_ids`, never genre names, so the detail request is what a lookup is really after. It appends `content_ratings` and `external_ids` in the same call, which keeps a full lookup at two requests. The `GET /genre/tv/list` table exists for the one case the detail request cannot cover — a series whose `genres` array comes back empty — and is fetched at most once every 24 hours into `internal/cache`.

TMDB v3 authenticates by query parameter, which has a consequence worth knowing about: `httpclient.APIError.Error()` prints the full request URL. The TMDB client therefore never forwards the underlying error. It rebuilds a message from the endpoint path, the status, and the response body, so a failed lookup cannot print the operator's API key into a log line. A test asserts exactly that.

### TheTVDB: one login, then a bearer

TheTVDB mints a bearer token from `POST /login` and requires it on every other route. The client logs in lazily on the first lookup and reuses the token for 24 hours — TheTVDB issues month-long tokens, so a conservative reuse window means a token can never be close to expiry in flight and the client needs no re-authenticate-and-retry path. Concurrent lookups share one login rather than each performing their own.

A subscriber (user-supported) key also needs a PIN; a project key omits it.

## Key sourcing

Provider keys are secrets and are treated like the Tunarr API key: supplied by the environment at wiring time, never written into a config file that lands in a Git repository. The clients enforce the half of that they can — `New` rejects an empty key and no client ever calls `os.Getenv` — so the choice of source stays with the caller, and a test can construct a client with an obviously-fake key without touching the environment.

## Genre vocabulary

`NormalizeGenre(raw)` folds one label into the vocabulary, ignoring case and surrounding whitespace, and reports `false` for anything with no canonical equivalent. `NormalizeGenres(list)` does the same for a list, preserving the source order and dropping both duplicates and unmapped labels — two raw labels can collapse onto one canonical name, and the first occurrence keeps its position. `CanonicalGenres()` returns the vocabulary itself.

The 23 canonical names are TMDB's movie genre list plus the television-only entries TMDB and TheTVDB add on top:

```txt
Action      Adventure   Animation   Comedy      Crime
Documentary Drama       Family      Fantasy     History
Horror      Music       Mystery     Romance     Science Fiction
Thriller    War         Western     Kids        News
Reality     Soap        Talk
```

Anything that is already one of those names matches directly. The mapping table covers the rest:

| Raw label | Canonical | Why |
| --- | --- | --- |
| `Action & Adventure` | Action | TMDB television genre 10759 is a compound with no plain "Action" twin. Action is the half an operator means; a source publishing `Adventure` alone still reaches Adventure directly. |
| `Sci-Fi & Fantasy` | Science Fiction | TMDB television genre 10765 fuses two vocabulary entries. Science Fiction is the more discriminating of the two. Known cost: a fantasy-only TMDB series normalizes to Science Fiction. TheTVDB, which publishes both separately, is unaffected. |
| `War & Politics` | War | TMDB television genre 10768. No canonical entry covers politics alone. |
| `Children` | Kids | TheTVDB's spelling of TMDB genre 10762. |
| `Anime` | Animation | An animation style, not a separate genre. A canonical entry for it would be unreachable from any non-TheTVDB source. |
| `Talk Show` | Talk | TheTVDB's spelling of TMDB genre 10767. |
| `Suspense` | Thriller | TheTVDB carries both; they describe the same territory. |
| `Game Show` | Reality | Unscripted competition programming. Reality is the only canonical entry covering unscripted television, and dropping it would leave game shows with no genre at all. |
| `Reality-TV`, `Soap Opera`, `Musical`, `Historical` | Reality, Soap, Music, History | Spellings library scrapers feeding Tunarr emit. |

Compounds collapse to a single canonical name rather than splitting into two. A genre filter answers "does this show carry genre X", so asserting a second genre the provider never published would make a filter match shows the operator did not ask for.

Some labels are deliberately unmapped, so `NormalizeGenre` reports `false` and they vanish from the result: TheTVDB's `Mini-Series` and TMDB's `TV Movie` describe a running format rather than a genre, and TheTVDB's `Food`, `Travel`, `Home and Garden`, `Sport`, and `Special Interest` have no canonical equivalent. Mapping any of them to a near-neighbor would be worse than dropping it.
