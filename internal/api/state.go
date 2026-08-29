package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// ListSeriesState implements gen.ServerInterface.
//
// store.ExportAllSeriesStates already returns exactly the wire shape this
// endpoint needs (every persisted series_state row, ordered by show_title),
// so this calls it directly rather than adding a same-shaped
// "ListSeriesStates" wrapper the brief floated as an option -- there is no
// name/behavior mismatch to paper over, only a rename, which isn't worth
// the extra indirection.
func (h *Handlers) ListSeriesState(w http.ResponseWriter, r *http.Request) {
	states, err := h.d.Store.ExportAllSeriesStates(r.Context())
	if err != nil {
		h.logAndWriteInternalError(w, r, "list_series_state", err)
		return
	}

	list := make([]gen.SeriesState, 0, len(states))
	for _, st := range states {
		list = append(list, seriesStateToGen(st))
	}
	writeJSON(w, http.StatusOK, list)
}

// PatchSeriesState implements gen.ServerInterface.
//
// Only fields present in the request body change (gen.SeriesStatePatch
// represents "not provided" as a nil pointer for each field); a body with
// no fields set is rejected with 400 rather than silently no-op'ing. An
// unknown show_title -- no persisted series_state row -- is rejected with
// 404: store.GetSeriesState is unsuitable for that check because it
// fabricates a default S01E01 state for any show, tracked or not (see its
// doc comment), so this uses store.GetPersistedSeriesState instead, which
// returns store.ErrNotFound when no row exists.
//
// After a successful update, every not-yet-aired occurrence snapshot for
// every block that references showTitle in its Series config is deleted
// (via invalidateSeriesOccurrenceSnapshots) -- otherwise this PATCH would
// be silently shadowed for up to the schedule-generation window (~a day):
// planSeriesOccurrences never re-reads series_state for an occurrence
// that already has a snapshot, so an operator's manual cursor reset would
// have no visible effect until every affected occurrence aged out of the
// window on its own. See StateStore.DeleteFutureOccurrenceSnapshots' doc
// comment.
func (h *Handlers) PatchSeriesState(w http.ResponseWriter, r *http.Request, showTitle string) {
	var patch gen.SeriesStatePatch
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&patch); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if patch.CurrentSeason == nil && patch.CurrentEpisode == nil && patch.Completed == nil && patch.Disabled == nil {
		WriteProblem(w, r, http.StatusBadRequest, "empty patch", "at least one field (current_season, current_episode, completed, disabled) must be set")
		return
	}

	current, err := h.d.Store.GetPersistedSeriesState(r.Context(), showTitle)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			WriteProblem(w, r, http.StatusNotFound, "series state not found", fmt.Sprintf("no tracked state for show %q", showTitle))
			return
		}
		h.logAndWriteInternalError(w, r, "patch_series_state_lookup", err)
		return
	}

	if patch.CurrentSeason != nil {
		current.CurrentSeason = *patch.CurrentSeason
	}
	if patch.CurrentEpisode != nil {
		current.CurrentEpisode = *patch.CurrentEpisode
	}
	if patch.Completed != nil {
		current.Completed = *patch.Completed
	}
	if patch.Disabled != nil {
		current.Disabled = *patch.Disabled
	}

	if err := h.d.Store.UpdateSeriesState(r.Context(), current); err != nil {
		h.logAndWriteInternalError(w, r, "patch_series_state_update", err)
		return
	}

	if err := h.invalidateSeriesOccurrenceSnapshots(r.Context(), showTitle); err != nil {
		h.logAndWriteInternalError(w, r, "patch_series_state_invalidate_snapshots", err)
		return
	}

	writeJSON(w, http.StatusOK, seriesStateToGen(*current))
}

// invalidateSeriesOccurrenceSnapshots deletes every not-yet-aired
// occurrence snapshot for every block whose Series config references
// showTitle -- see PatchSeriesState's doc comment for why. Matches by
// scanning every block's Spec.Series (there is no reverse index from
// show_title to block IDs; the blocks table is small enough -- one row
// per configured block -- that a full scan on an infrequent operator
// action like this is the right tradeoff over adding one).
func (h *Handlers) invalidateSeriesOccurrenceSnapshots(ctx context.Context, showTitle string) error {
	blocks, err := h.d.Store.ListBlocks(ctx)
	if err != nil {
		return fmt.Errorf("failed to list blocks while invalidating occurrence snapshots for %q: %w", showTitle, err)
	}

	now := time.Now()
	for _, b := range blocks {
		if !blockReferencesShow(b.Spec, showTitle) {
			continue
		}
		if err := h.d.Store.DeleteFutureOccurrenceSnapshots(ctx, b.ID, now); err != nil {
			return fmt.Errorf("failed to invalidate occurrence snapshots for block %q: %w", b.Name, err)
		}
	}
	return nil
}

// blockReferencesShow reports whether spec's Series config lists
// showTitle -- non-series blocks (an empty Series slice) trivially don't.
func blockReferencesShow(spec scheduler.Block, showTitle string) bool {
	for _, sc := range spec.Series {
		if sc.ShowTitle == showTitle {
			return true
		}
	}
	return false
}

// seriesStateToGen converts a scheduler.SeriesState (the persisted domain
// representation) into a gen.SeriesState (the API wire shape). ShowTitle,
// CurrentSeason, and CurrentEpisode are plain (non-pointer) fields on both
// sides; Completed, Disabled, and RunCount are pointers on the wire side
// (OpenAPI marks them optional) but always populated here since this only
// ever converts an already-persisted row.
func seriesStateToGen(s scheduler.SeriesState) gen.SeriesState {
	completed := s.Completed
	disabled := s.Disabled
	runCount := s.RunCount

	return gen.SeriesState{
		ShowTitle:      s.ShowTitle,
		CurrentSeason:  s.CurrentSeason,
		CurrentEpisode: s.CurrentEpisode,
		Completed:      &completed,
		Disabled:       &disabled,
		RunCount:       &runCount,
		LastAired:      s.LastAired,
	}
}
