package scheduler_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/scheduler"
)

func block(name, cron string, duration, overflow int) scheduler.Block {
	return scheduler.Block{
		Name:                       name,
		Cron:                       cron,
		Duration:                   duration,
		MaxDurationOverflowMinutes: overflow,
		ChannelID:                  "ch1",
		Type:                       scheduler.BlockTypeFilter,
	}
}

func TestOnAirOccurrences_FindsAnOccurrenceContainingNow(t *testing.T) {
	t.Parallel()
	// 21:00 daily, 3h. At 22:00 it is on air.
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	got := scheduler.OnAirOccurrences([]scheduler.Block{block("Late", "0 21 * * *", 180, 0)}, now, time.UTC)
	require.Len(t, got, 1)
	assert.Equal(t, "Late", got[0].Block.Name)
	assert.Equal(t, time.Date(2026, 6, 1, 21, 0, 0, 0, time.UTC), got[0].Start)
}

func TestOnAirOccurrences_IgnoresAnOccurrenceThatEnded(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 2, 5, 0, 0, 0, time.UTC)
	got := scheduler.OnAirOccurrences([]scheduler.Block{block("Late", "0 21 * * *", 180, 0)}, now, time.UTC)
	assert.Empty(t, got)
}

// The wider envelope, and the reason for it: store.InvalidationCutoff
// already treats a block's overflow as airing, and a guard that cleared
// before the invalidation path did would refuse-to-refuse on exactly the
// content that overran.
func TestOnAirOccurrences_CountsTheOverflowWindowAsAiring(t *testing.T) {
	t.Parallel()
	// 21:00 + 3h ends at 00:00. With 30 minutes of allowed overflow it is
	// still airing at 00:15.
	now := time.Date(2026, 6, 2, 0, 15, 0, 0, time.UTC)

	nominal := scheduler.OnAirOccurrences([]scheduler.Block{block("Late", "0 21 * * *", 180, 0)}, now, time.UTC)
	assert.Empty(t, nominal, "without overflow this occurrence is over")

	widened := scheduler.OnAirOccurrences([]scheduler.Block{block("Late", "0 21 * * *", 180, 30)}, now, time.UTC)
	require.Len(t, widened, 1, "the overflow window must count as airing")
}

func TestOnAirOccurrences_SafeAtIsTheEndOfTheWidenedEnvelope(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	got := scheduler.OnAirOccurrences([]scheduler.Block{block("Late", "0 21 * * *", 180, 30)}, now, time.UTC)
	require.Len(t, got, 1)
	// 21:00 + 180m + 30m
	assert.Equal(t, time.Date(2026, 6, 2, 0, 30, 0, 0, time.UTC), got[0].SafeAt)
	assert.True(t, got[0].SafeAt.After(now))
}

// The timezone precondition. A cron with no CRON_TZ prefix matches
// calendar fields against whatever location its argument carries, so a
// guard evaluating in UTC picks a different occurrence than an apply
// evaluating in Europe/Zurich.
func TestOnAirOccurrences_EvaluatesInTheGivenLocation(t *testing.T) {
	t.Parallel()
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	// 21:00 Zurich in June is 19:00Z. At 19:30Z the block IS airing in
	// Zurich terms, and is NOT airing if the cron is read as UTC.
	now := time.Date(2026, 6, 1, 19, 30, 0, 0, time.UTC)
	blocks := []scheduler.Block{block("Late", "0 21 * * *", 180, 0)}

	inZurich := scheduler.OnAirOccurrences(blocks, now, zurich)
	require.Len(t, inZurich, 1, "21:00 Zurich should be on air at 19:30Z")

	inUTC := scheduler.OnAirOccurrences(blocks, now, time.UTC)
	assert.Empty(t, inUTC, "read as UTC the same cron is not airing yet -- which is the bug")
}

// A filter block schedules titles that appear nowhere in its spec, so a
// guard driven off the show-reference lookup would miss it entirely. The
// predicate asks whether an OCCURRENCE is live, not how its content was
// chosen.
func TestOnAirOccurrences_CoversFilterBlocksNotJustSeries(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	filter := block("Late Movies", "0 21 * * *", 180, 0)
	require.Equal(t, scheduler.BlockTypeFilter, filter.Type)

	got := scheduler.OnAirOccurrences([]scheduler.Block{filter}, now, time.UTC)
	assert.Len(t, got, 1, "a filter block's occurrence is on air like any other")
}

func TestOnAirOccurrences_SkipsAnUnparseableCronRatherThanFailing(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	got := scheduler.OnAirOccurrences([]scheduler.Block{
		block("Bad", "not a cron", 180, 0),
		block("Good", "0 21 * * *", 180, 0),
	}, now, time.UTC)
	require.Len(t, got, 1, "one bad expression must not hide a live occurrence")
	assert.Equal(t, "Good", got[0].Block.Name)
}

func TestOnAirOccurrences_IgnoresANonPositiveDuration(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 22, 0, 0, 0, time.UTC)
	got := scheduler.OnAirOccurrences([]scheduler.Block{block("Zero", "0 21 * * *", 0, 0)}, now, time.UTC)
	assert.Empty(t, got)
}
