package radarr

import (
	"fmt"

	"github.com/geekxflood/schedularr/internal/tunarr"
)

// MoviesToPrograms converts Radarr movies into Tunarr programs, skipping entries without files.
func MoviesToPrograms(movies []Movie) []tunarr.Program {
	programs := make([]tunarr.Program, 0, len(movies))
	for _, movie := range movies {
		if !movie.HasFile || movie.Runtime <= 0 || movie.Title == "" {
			continue
		}
		programs = append(programs, tunarr.Program{
			ID:       fmt.Sprintf("radarr:%d", movie.ID),
			Title:    movie.Title,
			Year:     movie.Year,
			Summary:  movie.Overview,
			Duration: int64(movie.Runtime) * 60 * 1000,
			Rating:   movie.Certification,
			Genres:   movie.Genres,
			Type:     "movie",
		})
	}
	return programs
}
