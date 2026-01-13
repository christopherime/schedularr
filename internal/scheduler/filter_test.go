package scheduler

import (
	"testing"

	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterPrograms(t *testing.T) {
	programs := []tunarr.Program{
		{Title: "Movie A", Year: 2000, Genres: []string{"Action"}, Duration: 6000000}, // 100 min
		{Title: "Show B", Year: 2020, Genres: []string{"Comedy"}, Duration: 1800000},  // 30 min
	}

	f := Filter{
		Genres:      []string{"Action"},
		MinDuration: 90,
	}

	filtered, err := FilterPrograms(programs, f)
	require.NoError(t, err, "FilterPrograms returned error")
	require.Len(t, filtered, 1, "Expected 1 program")
	assert.Equal(t, "Movie A", filtered[0].Title)
}
