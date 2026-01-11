package scheduler

import (
	"regexp"
	"strings"

	"github.com/geekxflood/schedularr/internal/tunarr"
)

// FilterPrograms filters a list of programs based on the provided filter criteria.
func FilterPrograms(programs []tunarr.Program, f Filter) ([]tunarr.Program, error) {
	filtered := make([]tunarr.Program, 0, len(programs))

	titleRegex, err := compileTitlePattern(f.TitlePattern)
	if err != nil {
		return nil, err
	}

	for _, p := range programs {
		if !matchesFilter(p, f, titleRegex) {
			continue
		}
		filtered = append(filtered, p)
	}
	return filtered, nil
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

	if len(f.Genres) > 0 && !containsAny(p.Genres, f.Genres) {
		return false
	}

	if len(f.Ratings) > 0 && !contains(f.Ratings, p.Rating) {
		return false
	}

	if !matchesYearRange(p.Year, f.YearFrom, f.YearTo) {
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

func contains(slice []string, val string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}

func containsAny(source []string, targets []string) bool {
	for _, t := range targets {
		if contains(source, t) {
			return true
		}
	}
	return false
}
