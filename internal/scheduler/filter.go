package scheduler

import (
	"regexp"
	"strings"

	"github.com/geekxflood/schedularr/internal/tunarr"
)

// FilterPrograms filters a list of programs based on the provided filter criteria
func FilterPrograms(programs []tunarr.Program, f Filter) ([]tunarr.Program, error) {
	var filtered []tunarr.Program
	var titleRegex *regexp.Regexp
	var err error

	if f.TitlePattern != "" {
		titleRegex, err = regexp.Compile(f.TitlePattern)
		if err != nil {
			return nil, err
		}
	}

	for _, p := range programs {
		if titleRegex != nil && !titleRegex.MatchString(p.Title) {
			continue
		}

		if len(f.Genres) > 0 && !containsAny(p.Genres, f.Genres) {
			continue
		}

		if len(f.Ratings) > 0 && !contains(f.Ratings, p.Rating) {
			continue
		}

		if f.YearFrom > 0 && p.Year < f.YearFrom {
			continue
		}
		if f.YearTo > 0 && p.Year > f.YearTo {
			continue
		}

		durationMin := int(p.Duration / 60000) // convert ms to min
		if f.MinDuration > 0 && durationMin < f.MinDuration {
			continue
		}
		if f.MaxDuration > 0 && durationMin > f.MaxDuration {
			continue
		}

		filtered = append(filtered, p)
	}
	return filtered, nil
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
