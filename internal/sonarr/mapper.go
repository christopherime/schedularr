package sonarr

import (
	"fmt"

	"github.com/geekxflood/schedularr/internal/tunarr"
)

// EpisodesToPrograms converts Sonarr episodes into Tunarr programs, skipping entries without files.
func EpisodesToPrograms(seriesByID map[int]Series, episodes []Episode) []tunarr.Program {
	programs := make([]tunarr.Program, 0, len(episodes))
	for _, episode := range episodes {
		if !episode.HasFile {
			continue
		}

		series, ok := seriesByID[episode.SeriesID]
		if !ok || series.Title == "" {
			continue
		}

		duration := episode.Runtime
		if duration <= 0 {
			duration = series.Runtime
		}
		if duration <= 0 {
			continue
		}

		programs = append(programs, tunarr.Program{
			ID:        fmt.Sprintf("sonarr:%d", episode.ID),
			Title:     episode.Title,
			Year:      series.Year,
			Summary:   episode.Overview,
			Duration:  int64(duration) * 60 * 1000,
			Genres:    series.Genres,
			Type:      "episode",
			ShowTitle: series.Title,
			Season:    episode.SeasonNumber,
			Episode:   episode.EpisodeNumber,
		})
	}
	return programs
}
