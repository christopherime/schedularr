package scheduler

import (
	"time"
)

// OnAir is one occurrence that is playing right now.
type OnAir struct {
	// Block is the block whose occurrence this is.
	Block Block
	// Start is when the occurrence began.
	Start time.Time
	// SafeAt is when it stops being on air -- the instant after which a
	// change to this block can no longer affect what a viewer sees.
	SafeAt time.Time
}

// OnAirOccurrences returns every occurrence of blocks that is playing at
// now, each with the instant it becomes safe to change.
//
// It exists because three write paths -- deleting a block, switching one
// off, and (from v0.5.12) removing a show's history -- all change what the
// NEXT apply pushes to Tunarr, and the next apply may be minutes away and
// unattended: serve's cron loop applies at process start and then every
// cron_interval. None of them consulted the current lineup before this.
//
// THE ENVELOPE IS DELIBERATELY THE WIDER OF THE TWO IN THIS CODEBASE.
// onAirOccurrenceStart, which drives the planner's own on-air shell, uses
// Duration alone. store.InvalidationCutoff adds MaxDurationOverflowMinutes
// and its doc comment argues that a block whose content legitimately
// overruns IS still on air. This uses the second, because the same change
// that this guard protects also invalidates snapshots through that path --
// and a guard that cleared first would refuse to refuse on exactly the
// content that overran.
//
// THE LOCATION IS A PARAMETER, NOT time.Local. A block's cron carries no
// CRON_TZ prefix anywhere in this codebase, so robfig matches its calendar
// fields against whatever location the argument carries.
// GenerateForTimeRange converts to the engine's location before walking,
// and a guard that skipped that step would answer for a different
// occurrence than the apply it is protecting -- in a container with no TZ
// set, for a different hour entirely.
//
// IT ASKS ABOUT OCCURRENCES, NOT ABOUT CONTENT. Every block is checked,
// series and filter alike. A filter block schedules titles that appear
// nowhere in its spec, so anything driven off a show-reference lookup
// (store.blockReferencesShow scans spec.Series only) would silently miss
// every one of them.
//
// A block whose cron will not parse is skipped rather than failing the
// whole answer: one bad expression must not hide a live occurrence that a
// caller is about to disrupt. Callers pass the blocks they care about --
// for a guard, the ACTIVE ones, since a disabled or dark block generates
// no shell at the next apply and cannot be cut off by a change to it.
func OnAirOccurrences(blocks []Block, now time.Time, loc *time.Location) []OnAir {
	if loc == nil {
		loc = time.Local
	}
	local := now.In(loc)

	parser := NewCronParser()
	live := make([]OnAir, 0, len(blocks))
	for _, b := range blocks {
		schedule, err := parser.Parse(b.Cron)
		if err != nil {
			continue
		}
		envelope := time.Duration(b.Duration+b.MaxDurationOverflowMinutes) * time.Minute
		start, ok := onAirOccurrenceStart(schedule, envelope, local)
		if !ok {
			continue
		}
		live = append(live, OnAir{Block: b, Start: start, SafeAt: start.Add(envelope)})
	}
	return live
}
