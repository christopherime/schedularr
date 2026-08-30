// Package metadata looks up show metadata at external providers and
// folds their differing genre labels into one canonical vocabulary, so
// that a block's filter.genres means the same thing no matter which
// source a Tunarr library was built from.
//
// The package is provider-agnostic. internal/metadata/tmdb and
// internal/metadata/tvdb implement Provider; neither reads the
// environment. API keys arrive through a constructor, sourced by the
// caller from config or a secret at wiring time.
package metadata

import (
	"context"
	"errors"
)

// ErrNotFound reports that a title has no match at a provider. It is an
// ordinary outcome of a lookup pass -- a caller checks it with
// errors.Is and moves on to the next show rather than failing the whole
// pass.
var ErrNotFound = errors.New("show not found")

// ErrUnauthorized reports that a provider rejected the API key the
// client was built with. Unlike ErrNotFound it is fatal to a lookup
// pass: every later call carrying the same key fails the same way, so a
// caller aborts instead of recording hundreds of shows as unmatched.
var ErrUnauthorized = errors.New("provider rejected the api key")

// ShowMetadata is one provider's answer about one show, in a shape that
// does not vary by provider.
type ShowMetadata struct {
	// Title is the provider's own name for the show, which may differ
	// from the title that was searched for.
	Title string

	// Year is the first-air year, or 0 when the provider gave none.
	Year int

	// Genres are already normalized to the canonical vocabulary by
	// NormalizeGenres: deduplicated, source order preserved, and labels
	// with no canonical equivalent dropped.
	Genres []string

	// Rating is the content rating (TV-MA, PG-13), preferring the US
	// certification because that is the vocabulary Tunarr libraries
	// carry. Empty when the provider published none.
	Rating string

	// Overview is the provider's synopsis.
	Overview string

	// PosterURL is an absolute URL to poster art, empty when the
	// provider has none.
	PosterURL string

	// ExternalIDs maps a source name to this show's ID at that source.
	// It always carries the answering provider's own Name(), plus any
	// cross-references that provider publishes ("imdb", "tmdb",
	// "tvdb").
	ExternalIDs map[string]string
}

// Provider is one external metadata source.
type Provider interface {
	// Name returns the provider's short identifier ("tmdb", "tvdb"),
	// which is also the key it sets in ShowMetadata.ExternalIDs.
	Name() string

	// LookupShow resolves a show by title. A year of 0 means "no hint";
	// a non-zero year narrows the search and breaks ties between
	// same-named shows. It returns an error wrapping ErrNotFound when
	// the provider has no match, and one wrapping ErrUnauthorized when
	// the provider rejected the key.
	LookupShow(ctx context.Context, title string, year int) (*ShowMetadata, error)
}
