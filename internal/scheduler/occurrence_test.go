package scheduler_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/scheduler"
)

func TestNextOccurrencesReturnsCountStartsAfterFrom(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("0 6 * * *", from, 3)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	want := []time.Time{
		time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 12, 6, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 13, 6, 0, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNextOccurrencesIsStrictlyAfterFrom(t *testing.T) {
	t.Parallel()
	// from sits exactly on an occurrence: it must not be returned again.
	from := time.Date(2026, 9, 10, 6, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("0 6 * * *", from, 1)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	want := time.Date(2026, 9, 11, 6, 0, 0, 0, time.UTC)
	if len(got) != 1 || !got[0].Equal(want) {
		t.Fatalf("got %v, want [%v]", got, want)
	}
}

func TestNextOccurrencesEvaluatesInFromsLocation(t *testing.T) {
	t.Parallel()
	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, zurich)
	got, err := scheduler.NextOccurrences("0 6 * * *", from, 1)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	if got[0].Location() != zurich {
		t.Fatalf("location = %v, want %v", got[0].Location(), zurich)
	}
	if h := got[0].Hour(); h != 6 {
		t.Fatalf("hour = %d, want 6 local", h)
	}
}

func TestNextOccurrencesShortensWhenTheScheduleRunsOut(t *testing.T) {
	t.Parallel()
	// February 30th never happens. robfig returns the zero time rather
	// than an error, which must end the walk and never reach a caller.
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("0 0 30 2 *", from, 3)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want no occurrences", got)
	}
	for _, o := range got {
		if o.IsZero() {
			t.Fatal("a zero time reached the caller")
		}
	}
}

func TestNextOccurrencesRejectsAnUnparseableExpression(t *testing.T) {
	t.Parallel()
	_, err := scheduler.NextOccurrences("not a cron", time.Now(), 1)
	if err == nil {
		t.Fatal("expected an error for an unparseable expression")
	}
}

func TestNextOccurrencesRejectsANonPositiveCount(t *testing.T) {
	t.Parallel()
	if _, err := scheduler.NextOccurrences("0 6 * * *", time.Now(), 0); err == nil {
		t.Fatal("expected an error for count 0")
	}
}

func TestNextOccurrencesAcceptsDescriptors(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("@daily", from, 2)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d occurrences, want 2", len(got))
	}
	if got[0].Hour() != 0 {
		t.Fatalf("@daily fired at %v, want midnight", got[0])
	}
}

// TestNextOccurrencesEveryDescriptorIsRelativeToItsArgument pins a robfig
// behaviour that reads like a bug and is not: `@every` yields a
// ConstantDelaySchedule measured from the instant handed in, so it walks
// 90-minute steps off `from` rather than snapping to a calendar boundary.
// Anyone "fixing" that would break every @every block in the store.
func TestNextOccurrencesEveryDescriptorIsRelativeToItsArgument(t *testing.T) {
	t.Parallel()
	from := time.Date(2026, 9, 10, 12, 17, 0, 0, time.UTC)
	got, err := scheduler.NextOccurrences("@every 1h30m", from, 2)
	if err != nil {
		t.Fatalf("NextOccurrences: %v", err)
	}
	want := []time.Time{
		time.Date(2026, 9, 10, 13, 47, 0, 0, time.UTC),
		time.Date(2026, 9, 10, 15, 17, 0, 0, time.UTC),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v -- @every is relative to its argument, not to a calendar", got, want)
	}
}

// TestNextOccurrencesRejectsACountAboveTheCeiling pins the upper guard, so
// a caller cannot make one request walk a five-year search thousands of
// times.
func TestNextOccurrencesRejectsACountAboveTheCeiling(t *testing.T) {
	t.Parallel()
	if _, err := scheduler.NextOccurrences("0 6 * * *", time.Now(), 101); err == nil {
		t.Fatal("expected an error for a count above the ceiling")
	}
}
