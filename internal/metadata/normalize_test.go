package metadata

import (
	"reflect"
	"slices"
	"testing"
)

// TestNormalizeGenre walks the whole shape of the mapping: exact
// canonical names, the case/whitespace insensitivity every provider
// needs (TheTVDB emits "Science Fiction", scrapers emit "science
// fiction"), the compound TMDB television labels that have no canonical
// twin, the TheTVDB-only spellings, and the labels the vocabulary
// deliberately has no place for.
func TestNormalizeGenre(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "canonical name passes through", raw: "Drama", want: "Drama", ok: true},
		{name: "lowercase canonical name", raw: "comedy", want: "Comedy", ok: true},
		{name: "uppercase canonical name", raw: "WESTERN", want: "Western", ok: true},
		{name: "multi-word canonical name", raw: "science fiction", want: "Science Fiction", ok: true},
		{name: "surrounding whitespace is trimmed", raw: "  Horror \t", want: "Horror", ok: true},

		// TMDB television compounds.
		{name: "tmdb action & adventure", raw: "Action & Adventure", want: "Action", ok: true},
		{name: "tmdb sci-fi & fantasy", raw: "Sci-Fi & Fantasy", want: "Science Fiction", ok: true},
		{name: "tmdb war & politics", raw: "War & Politics", want: "War", ok: true},
		{name: "tmdb kids", raw: "Kids", want: "Kids", ok: true},
		{name: "tmdb soap", raw: "Soap", want: "Soap", ok: true},
		{name: "tmdb talk", raw: "Talk", want: "Talk", ok: true},

		// TheTVDB spellings.
		{name: "tvdb children", raw: "Children", want: "Kids", ok: true},
		{name: "tvdb anime", raw: "Anime", want: "Animation", ok: true},
		{name: "tvdb talk show", raw: "Talk Show", want: "Talk", ok: true},
		{name: "tvdb suspense", raw: "Suspense", want: "Thriller", ok: true},
		{name: "tvdb game show", raw: "Game Show", want: "Reality", ok: true},
		{name: "tvdb science fiction is already canonical", raw: "Science Fiction", want: "Science Fiction", ok: true},

		// Scraper spellings.
		{name: "reality-tv", raw: "Reality-TV", want: "Reality", ok: true},
		{name: "soap opera", raw: "Soap Opera", want: "Soap", ok: true},
		{name: "musical", raw: "Musical", want: "Music", ok: true},
		{name: "historical", raw: "Historical", want: "History", ok: true},

		// Deliberately unmapped: a format, not a genre.
		{name: "tvdb mini-series is a format", raw: "Mini-Series", want: "", ok: false},
		{name: "tmdb tv movie is a format", raw: "TV Movie", want: "", ok: false},

		// Deliberately unmapped: no canonical equivalent.
		{name: "tvdb sport", raw: "Sport", want: "", ok: false},
		{name: "tvdb travel", raw: "Travel", want: "", ok: false},
		{name: "tvdb food", raw: "Food", want: "", ok: false},
		{name: "tvdb home and garden", raw: "Home and Garden", want: "", ok: false},
		{name: "tvdb special interest", raw: "Special Interest", want: "", ok: false},

		{name: "unknown label", raw: "Competitive Cheese Rolling", want: "", ok: false},
		{name: "empty string", raw: "", want: "", ok: false},
		{name: "whitespace only", raw: "   ", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeGenre(tt.raw)
			if ok != tt.ok {
				t.Fatalf("NormalizeGenre(%q) ok = %v, want %v (got %q)", tt.raw, ok, tt.ok, got)
			}
			if got != tt.want {
				t.Errorf("NormalizeGenre(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

// TestNormalizeGenres pins the list-level contract: source order
// survives, duplicates collapse (including two different raw labels that
// map to one canonical name), unmapped labels vanish, and a list that
// yields nothing comes back nil rather than an empty slice.
func TestNormalizeGenres(t *testing.T) {
	tests := []struct {
		name string
		raw  []string
		want []string
	}{
		{
			name: "source order is preserved",
			raw:  []string{"Drama", "Crime", "Thriller"},
			want: []string{"Drama", "Crime", "Thriller"},
		},
		{
			name: "two raw labels mapping to one canonical name collapse",
			raw:  []string{"Anime", "Animation"},
			want: []string{"Animation"},
		},
		{
			name: "the first occurrence keeps its position",
			raw:  []string{"Suspense", "Comedy", "Thriller"},
			want: []string{"Thriller", "Comedy"},
		},
		{
			name: "unmapped labels are dropped, the rest survive",
			raw:  []string{"Mini-Series", "Drama", "Sport"},
			want: []string{"Drama"},
		},
		{
			name: "a tmdb television genre list normalizes whole",
			raw:  []string{"Action & Adventure", "Sci-Fi & Fantasy", "Drama"},
			want: []string{"Action", "Science Fiction", "Drama"},
		},
		{
			name: "nothing mappable yields nil",
			raw:  []string{"Sport", "Travel"},
			want: nil,
		},
		{
			name: "empty input yields nil",
			raw:  nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeGenres(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NormalizeGenres(%v) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

// TestGenreAliasesAreWellFormed guards the mapping table itself rather
// than any one lookup: an alias whose target is not in the vocabulary
// would leak a non-canonical genre out of NormalizeGenre, an alias key
// that is already a canonical name would be dead (the exact-match path
// wins first), and an alias key that is not lowercased could never be
// hit, since NormalizeGenre lowercases before looking one up.
func TestGenreAliasesAreWellFormed(t *testing.T) {
	vocabulary := CanonicalGenres()

	for alias, target := range genreAliases {
		if !slices.Contains(vocabulary, target) {
			t.Errorf("alias %q targets %q, which is not in the canonical vocabulary", alias, target)
		}
		if _, canonical := canonicalByLower[alias]; canonical {
			t.Errorf("alias %q duplicates a canonical name and can never be reached", alias)
		}
		if got, _ := NormalizeGenre(alias); got != target {
			t.Errorf("NormalizeGenre(%q) = %q, want the alias target %q", alias, got, target)
		}
	}
}

// TestCanonicalGenresReturnsACopy pins that a caller sorting or
// truncating the vocabulary cannot corrupt the package's own table.
func TestCanonicalGenresReturnsACopy(t *testing.T) {
	first := CanonicalGenres()
	if len(first) == 0 {
		t.Fatal("CanonicalGenres returned an empty vocabulary")
	}

	first[0] = "Mutated"
	slices.Sort(first)

	second := CanonicalGenres()
	if second[0] != canonicalGenres[0] {
		t.Errorf("CanonicalGenres was mutated through a returned slice: got %q, want %q", second[0], canonicalGenres[0])
	}
	if slices.Contains(second, "Mutated") {
		t.Error("CanonicalGenres leaked a caller's mutation back into the vocabulary")
	}
}
