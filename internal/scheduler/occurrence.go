package scheduler

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// maxOccurrences bounds one NextOccurrences call.
//
// No caller in this tree can reach it: GET /cron/next range-checks count to
// [1,10] before calling, and toGen asks for exactly 1. It is kept anyway
// because this is an EXPORTED function and count is caller-supplied -- each
// step is a five-year calendar search, so an unbounded count is a hang, not
// a slow answer. The endpoint's 1..10 is a contract promise; this is what
// the function itself will agree to do.
const maxOccurrences = 100

// NewCronParser builds the project's ONE cron parser configuration:
// standard 5-field expressions plus descriptors (@daily, @every 1h30m).
//
// It exists so the engine and NextOccurrences cannot drift apart about
// what a valid expression is. A block whose cron the planner accepts must
// be one the UI can show occurrences for, and the reverse -- two parsers
// with different option sets would produce exactly that lie.
func NewCronParser() cron.Parser {
	return cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

// NextOccurrences returns the next `count` start instants strictly after
// `from`, in from's own location.
//
// Strictly after, so a `from` that sits exactly on an occurrence does not
// return that occurrence again -- the caller is asking what happens NEXT.
// This is deliberately a different convention from the engine's window
// walk, which seeds at from-1s precisely so an occurrence exactly at the
// window start IS included; a window is inclusive of its edge, "next" is
// not.
//
// The result may be SHORTER than count. robfig's SpecSchedule.Next returns
// the zero time when it finds no match within five years -- February 30th
// is a well-formed expression that never fires -- so the walk stops there
// rather than letting a zero instant reach a caller and serialize as
// 0001-01-01.
//
// A @every descriptor yields a ConstantDelaySchedule measured from the
// argument rather than from a calendar. That is the library's intent, not
// a bug to correct.
func NextOccurrences(expr string, from time.Time, count int) ([]time.Time, error) {
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive, got %d", count)
	}
	if count > maxOccurrences {
		return nil, fmt.Errorf("count must be at most %d, got %d", maxOccurrences, count)
	}

	schedule, err := NewCronParser().Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cron %q: %w", expr, err)
	}

	out := make([]time.Time, 0, count)
	next := from
	for range count {
		next = schedule.Next(next)
		if next.IsZero() {
			break
		}
		out = append(out, next)
	}
	return out, nil
}
