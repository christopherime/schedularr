package metadata

import "strings"

// canonicalGenres is the closed vocabulary every provider label is
// folded into. It is the union of TMDB's movie genre list and the
// television-only entries TMDB and TheTVDB add on top (Kids, News,
// Reality, Soap, Talk), which between them cover every genre either
// provider can return. Nothing outside this list ever reaches a
// ShowMetadata.
var canonicalGenres = []string{
	"Action",
	"Adventure",
	"Animation",
	"Comedy",
	"Crime",
	"Documentary",
	"Drama",
	"Family",
	"Fantasy",
	"History",
	"Horror",
	"Music",
	"Mystery",
	"Romance",
	"Science Fiction",
	"Thriller",
	"War",
	"Western",
	"Kids",
	"News",
	"Reality",
	"Soap",
	"Talk",
}

// canonicalByLower resolves a lowercased label that is already a
// canonical name. Case-insensitive exact matches are handled here, so
// genreAliases below never has to repeat the vocabulary itself.
var canonicalByLower = func() map[string]string {
	m := make(map[string]string, len(canonicalGenres))
	for _, g := range canonicalGenres {
		m[strings.ToLower(g)] = g
	}
	return m
}()

// genreAliases maps a lowercased provider label that is not itself a
// canonical name onto the canonical name closest to it. Compound labels
// collapse to a single entry rather than splitting: a genre filter
// answers "does this show carry genre X", and inventing a second genre
// the provider never asserted would make a filter match shows the
// operator did not ask for.
//
// Each non-obvious mapping carries its reason.
var genreAliases = map[string]string{
	// TMDB's television list has no plain "Action": genre 10759 is the
	// compound "Action & Adventure". Action is the half an operator
	// means when filtering for it, and a source that publishes
	// "Adventure" on its own still reaches the Adventure entry through
	// the exact-match path.
	"action & adventure":   "Action",
	"action and adventure": "Action",

	// TMDB television genre 10765 fuses two separate vocabulary
	// entries. Science Fiction wins because it is the more
	// discriminating of the two. The known cost is that a
	// fantasy-only TMDB series normalizes to Science Fiction; TheTVDB,
	// which publishes the two separately, is unaffected.
	"sci-fi & fantasy":          "Science Fiction",
	"sci-fi and fantasy":        "Science Fiction",
	"science fiction & fantasy": "Science Fiction",
	"sci-fi":                    "Science Fiction",
	"scifi":                     "Science Fiction",
	"science-fiction":           "Science Fiction",

	// TMDB television genre 10768 is "War & Politics". No canonical
	// entry covers politics on its own, so the whole label lands on
	// War.
	"war & politics":   "War",
	"war and politics": "War",

	// TheTVDB labels the children's category "Children"; TMDB calls the
	// same thing "Kids" (genre 10762).
	"children":   "Kids",
	"childrens":  "Kids",
	"children's": "Kids",

	// Anime is an animation style, not a separate genre, and adding a
	// canonical entry for it would leave every non-TheTVDB source
	// unable to produce it.
	"anime":   "Animation",
	"cartoon": "Animation",

	// TheTVDB says "Talk Show" where TMDB says "Talk" (genre 10767).
	"talk show":  "Talk",
	"talk shows": "Talk",

	// TheTVDB carries Suspense as a genre of its own; it describes the
	// same territory as Thriller, which both providers also use.
	"suspense": "Thriller",

	// TheTVDB's "Game Show" is unscripted competition programming, and
	// Reality is the only canonical entry that covers unscripted
	// television. The alternative -- dropping it -- would leave game
	// shows with no genre at all.
	"game show":  "Reality",
	"game shows": "Reality",

	// Common spellings of the unscripted and serial-drama categories
	// seen on library scrapers feeding Tunarr.
	"reality-tv":  "Reality",
	"reality tv":  "Reality",
	"soap opera":  "Soap",
	"soap operas": "Soap",

	// TMDB uses "Music"; scrapers and TheTVDB both emit "Musical" for
	// the same category.
	"musical": "Music",

	// Adjectival spelling of the History entry.
	"historical": "History",

	// Deliberately absent, so that NormalizeGenre reports false for
	// them: TheTVDB's "Mini-Series" and TMDB's "TV Movie" describe a
	// running format rather than a genre, and TheTVDB's "Food",
	// "Travel", "Home and Garden", "Sport", and "Special Interest"
	// have no canonical equivalent. Mapping any of them to a
	// near-neighbor would make a genre filter match shows an operator
	// never asked for.
}

// NormalizeGenre folds one raw provider label into the canonical
// vocabulary. Matching ignores case and surrounding whitespace.
//
// It reports false for a label the vocabulary has no place for --
// including the format labels and niche categories listed at the end of
// genreAliases. Callers drop those rather than inventing a genre an
// operator cannot filter on.
func NormalizeGenre(raw string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "", false
	}
	if canonical, ok := canonicalByLower[key]; ok {
		return canonical, true
	}
	canonical, ok := genreAliases[key]
	return canonical, ok
}

// NormalizeGenres folds a provider's genre list into the canonical
// vocabulary, preserving the source order and dropping both duplicates
// (two raw labels can share one canonical name) and labels
// NormalizeGenre rejects. It returns nil when nothing survives, so an
// unmatched list and an empty one are indistinguishable to a caller.
func NormalizeGenres(raw []string) []string {
	normalized := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))

	for _, r := range raw {
		canonical, ok := NormalizeGenre(r)
		if !ok {
			continue
		}
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		normalized = append(normalized, canonical)
	}

	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

// CanonicalGenres returns the vocabulary NormalizeGenre maps into, as a
// fresh copy the caller may sort or filter freely.
func CanonicalGenres() []string {
	out := make([]string, len(canonicalGenres))
	copy(out, canonicalGenres)
	return out
}
