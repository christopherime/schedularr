package scheduler

import (
	"regexp"
	"strings"

	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/samber/lo"
)

// FilterPrograms filters a list of programs based on the provided filter criteria.
func FilterPrograms(programs []tunarr.Program, f Filter) ([]tunarr.Program, error) {
	titleRegex, err := compileTitlePattern(f.TitlePattern)
	if err != nil {
		return nil, err
	}

	return lo.Filter(programs, func(p tunarr.Program, _ int) bool {
		return matchesFilter(p, f, titleRegex)
	}), nil
}

func compileTitlePattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, nil
	}
	return regexp.Compile(pattern)
}

func matchesFilter(p tunarr.Program, f Filter, titleRegex *regexp.Regexp) bool {
	if titleRegex != nil && !titleRegex.MatchString(p.Title) {
		return false
	}

	if len(f.Genres) > 0 && !containsAnyIgnoreCase(p.GetGenreNames(), f.Genres) {
		return false
	}

	if len(f.Ratings) > 0 && !containsIgnoreCase(f.Ratings, p.Rating) {
		return false
	}

	if !matchesYearRange(p.GetYear(), f.YearFrom, f.YearTo) {
		return false
	}

	durationMin := int(p.Duration / 60000) // convert ms to min
	return matchesDurationRange(durationMin, f.MinDuration, f.MaxDuration)
}

func matchesYearRange(year, yearFrom, yearTo int) bool {
	if yearFrom > 0 && year < yearFrom {
		return false
	}
	if yearTo > 0 && year > yearTo {
		return false
	}
	return true
}

func matchesDurationRange(duration, minDuration, maxDuration int) bool {
	if minDuration > 0 && duration < minDuration {
		return false
	}
	if maxDuration > 0 && duration > maxDuration {
		return false
	}
	return true
}

// containsIgnoreCase checks if slice contains val (case-insensitive).
func containsIgnoreCase(slice []string, val string) bool {
	return lo.ContainsBy(slice, func(item string) bool {
		return strings.EqualFold(item, val)
	})
}

// containsAnyIgnoreCase checks if any of the targets are in source (case-insensitive).
func containsAnyIgnoreCase(source []string, targets []string) bool {
	return lo.SomeBy(targets, func(t string) bool {
		return containsIgnoreCase(source, t)
	})
}
