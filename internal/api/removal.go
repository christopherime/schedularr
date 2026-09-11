package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/events"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// RemoveSeriesState implements gen.ServerInterface.
//
// Removing a show's progression is the one desk action that cannot be
// undone, so it is a two-step flow by construction: dry_run reports what
// would go, and only a second call without it removes anything.
//
// THE 409 IS THE INTERESTING PATH. store.RemoveShow refuses while any
// block still lists the show, because the next apply would re-add it from
// the block spec and reset the cursor the removal just deleted. The block
// names go in the problem's detail as prose, matching refuseIfOnAir's
// precedent and for its reason: problem.Problem is a closed struct shared
// with internal/api/middleware, and widening it for one caller is a worse
// trade than a sentence. The desk does not parse that sentence -- it
// reads GET /blocks itself and shows the removal as blocked before the
// operator ever clicks, so this is the backstop for a block added between
// that check and the delete.
func (h *Handlers) RemoveSeriesState(w http.ResponseWriter, r *http.Request, showTitle string, params gen.RemoveSeriesStateParams) {
	// GetSeriesState fabricates a default S01E01 for any title, so the
	// existence check has to go through the persisted read -- the same
	// reason PatchSeriesState uses it.
	if _, err := h.d.Store.GetPersistedSeriesState(r.Context(), showTitle); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			WriteProblem(w, r, http.StatusNotFound, "series state not found",
				fmt.Sprintf("no tracked state for show %q", showTitle))
			return
		}
		h.logAndWriteInternalError(w, r, "remove_series_state_lookup", err)
		return
	}

	if isDryRun(params.DryRun) {
		report, err := h.d.Store.CountShowRemoval(r.Context(), showTitle)
		if err != nil {
			h.writeRemovalError(w, r, "remove_series_state_dry_run", err)
			return
		}
		writeJSON(w, http.StatusOK, removalReportToGen(report, true))
		return
	}

	report, err := h.d.Store.RemoveShow(r.Context(), showTitle)
	if err != nil {
		h.writeRemovalError(w, r, "remove_series_state", err)
		return
	}

	// The cursor and the airings both moved, so both panes are stale.
	h.publish(events.SeriesChanged, map[string]any{"show_title": showTitle})
	h.publishPlanInvalidated("series", showTitle)

	writeJSON(w, http.StatusOK, removalReportToGen(report, false))
}

// DeleteHistoryRange implements gen.ServerInterface.
func (h *Handlers) DeleteHistoryRange(w http.ResponseWriter, r *http.Request, params gen.DeleteHistoryRangeParams) {
	window, err := historyRangeFrom(params)
	if err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "invalid range", err.Error())
		return
	}

	if !h.refuseIfRangeIsOnAir(w, r, window) {
		return
	}

	if isDryRun(params.DryRun) {
		report, err := h.d.Store.CountHistoryRange(r.Context(), window)
		if err != nil {
			h.writeRemovalError(w, r, "delete_history_range_dry_run", err)
			return
		}
		writeJSON(w, http.StatusOK, removalReportToGen(report, true))
		return
	}

	report, err := h.d.Store.DeleteHistoryRange(r.Context(), window)
	if err != nil {
		h.writeRemovalError(w, r, "delete_history_range", err)
		return
	}

	h.publishPlanInvalidated("history", "range")
	writeJSON(w, http.StatusOK, removalReportToGen(report, false))
}

// GetStorage implements gen.ServerInterface.
func (h *Handlers) GetStorage(w http.ResponseWriter, r *http.Request) {
	counts, err := h.d.Store.StorageCounts(r.Context())
	if err != nil {
		h.logAndWriteInternalError(w, r, "get_storage", err)
		return
	}
	writeJSON(w, http.StatusOK, gen.StorageReport{
		SeriesStates: int(counts.SeriesStates),
		Airings:      int(counts.Airings),
		Sentinels:    int(counts.Sentinels),
		Snapshots:    int(counts.Snapshots),
		ApplyRuns:    int(counts.ApplyRuns),
		Warnings:     int(counts.Warnings),
		OldestAiring: counts.OldestAiring,
		NewestAiring: counts.NewestAiring,
	})
}

// historyRangeFrom reads the two mutually exclusive window forms.
//
// Exactly one is required. Accepting neither would make an empty query
// string mean "delete every airing", which is how an accident happens;
// accepting both would leave the precedence rule invisible to whoever
// sent them.
func historyRangeFrom(params gen.DeleteHistoryRangeParams) (store.HistoryRange, error) {
	hasBefore := params.Before != nil
	hasSpan := params.From != nil || params.To != nil

	switch {
	case hasBefore && hasSpan:
		return store.HistoryRange{}, errors.New("send before, or from/to, not both")
	case !hasBefore && !hasSpan:
		return store.HistoryRange{}, errors.New("send before=<instant>, or from=<instant>&to=<instant>")
	}

	window := store.HistoryRange{
		ChannelID: derefOr(params.ChannelId),
		BlockName: derefOr(params.BlockName),
		ShowTitle: derefOr(params.ShowTitle),
	}
	if hasBefore {
		window.To = *params.Before
		return window, nil
	}
	if params.From != nil {
		window.From = *params.From
	}
	if params.To != nil {
		window.To = *params.To
	}
	return window, nil
}

// refuseIfRangeIsOnAir answers 409 when any enabled block has an
// occurrence playing right now whose airings the window would delete.
//
// Deleting the record of what is on air is the one deletion that changes
// what a viewer sees: the occurrence loses its committed content, and the
// next apply -- which serve's cron loop runs unattended -- re-plans the
// slot mid-program. The sentinel keeps an EMPTIED occurrence committed,
// but an occurrence that is still airing has not finished producing the
// content the sentinel would claim it never produced.
//
// It reuses scheduler.OnAirOccurrences rather than re-deriving the
// envelope: that function's doc comment is explicit that the wider of the
// codebase's two envelopes is deliberate, and a second copy would drift
// from it.
func (h *Handlers) refuseIfRangeIsOnAir(w http.ResponseWriter, r *http.Request, window store.HistoryRange) bool {
	records, err := h.d.Store.ListBlocks(r.Context())
	if err != nil {
		h.logAndWriteInternalError(w, r, "delete_history_range_blocks", err)
		return false
	}

	now := time.Now()
	active := make([]scheduler.Block, 0, len(records))
	for _, rec := range records {
		if !rec.Enabled || (rec.DisabledUntil != nil && now.Before(*rec.DisabledUntil)) {
			continue
		}
		if window.BlockName != "" && rec.Name != window.BlockName {
			continue
		}
		if window.ChannelID != "" && rec.Spec.ChannelID != window.ChannelID {
			continue
		}
		active = append(active, rec.Spec)
	}

	for _, live := range scheduler.OnAirOccurrences(active, now, h.location()) {
		if !windowCovers(window, live.Start) {
			continue
		}
		safeAt := live.SafeAt.In(h.location())
		WriteProblem(w, r, http.StatusConflict, "range covers what is on air",
			fmt.Sprintf("%s is airing until %s. Deleting its airings now would cut the current program. Try again after that.",
				live.Block.Name, safeAt.Format("15:04")))
		return false
	}
	return true
}

// windowCovers reports whether an occurrence's start falls inside the
// window. A zero bound is open on that side, matching HistoryRange.
func windowCovers(window store.HistoryRange, start time.Time) bool {
	if !window.From.IsZero() && start.Before(window.From) {
		return false
	}
	if !window.To.IsZero() && !start.Before(window.To) {
		return false
	}
	return true
}

// writeRemovalError maps the store's removal failures onto responses.
// ErrShowStillScheduled is a 409 naming the blocks to edit first; an
// unbounded or backward range is the caller's mistake, not ours.
func (h *Handlers) writeRemovalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	var scheduled *store.ShowStillScheduledError
	if errors.As(err, &scheduled) {
		WriteProblem(w, r, http.StatusConflict, "show is still scheduled",
			fmt.Sprintf("%s is still listed by %s. Remove it from %s first, or the next apply would re-add it and reset the cursor this would delete.",
				scheduled.ShowTitle, strings.Join(scheduled.BlockNames, ", "),
				plural(len(scheduled.BlockNames), "that block", "those blocks")))
		return
	}
	if errors.Is(err, store.ErrUnboundedRange) || errors.Is(err, store.ErrBackwardRange) {
		WriteProblem(w, r, http.StatusBadRequest, "invalid range", err.Error())
		return
	}
	h.logAndWriteInternalError(w, r, op, err)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func isDryRun(v *bool) bool { return v != nil && *v }

func derefOr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func removalReportToGen(report store.RemovalReport, dryRun bool) gen.RemovalReport {
	seriesStates := int(report.SeriesStates)
	snapshots := int(report.Snapshots)
	return gen.RemovalReport{
		SeriesStates: &seriesStates,
		Airings:      int(report.Airings),
		Snapshots:    &snapshots,
		EmptiedSlots: int(report.EmptiedSlots),
		DryRun:       &dryRun,
	}
}
