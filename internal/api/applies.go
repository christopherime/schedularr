package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/store"
)

// Window and page bounds for GET /applies. These mirror the OpenAPI
// schema (api/openapi.yaml), which oapi-codegen's chi-server generator
// does not enforce at the binding layer -- see GetHistory's doc comment
// in history.go for the full explanation of why every query parameter is
// re-validated here rather than trusted from the generated wrapper.
const (
	defaultApplyDays  = 7
	minApplyDays      = 1
	maxApplyDays      = 90
	defaultApplyLimit = 100
	minApplyLimit     = 1
	maxApplyLimit     = 500
)

// ListApplyRuns implements gen.ServerInterface.
func (h *Handlers) ListApplyRuns(w http.ResponseWriter, r *http.Request, params gen.ListApplyRunsParams) {
	days := defaultApplyDays
	if params.Days != nil {
		days = *params.Days
	}
	if days < minApplyDays || days > maxApplyDays {
		WriteProblem(w, r, http.StatusBadRequest, "invalid days parameter",
			fmt.Sprintf("days must be between %d and %d, got %d", minApplyDays, maxApplyDays, days))
		return
	}

	limit := defaultApplyLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	if limit < minApplyLimit || limit > maxApplyLimit {
		WriteProblem(w, r, http.StatusBadRequest, "invalid limit parameter",
			fmt.Sprintf("limit must be between %d and %d, got %d", minApplyLimit, maxApplyLimit, limit))
		return
	}

	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	runs, err := h.d.Store.ListApplyRuns(r.Context(), since, limit)
	if err != nil {
		h.logAndWriteInternalError(w, r, "list_apply_runs", err)
		return
	}

	list := make([]gen.ApplyRun, 0, len(runs))
	for _, run := range runs {
		list = append(list, applyRunToGen(run))
	}
	writeJSON(w, http.StatusOK, list)
}

// applyRunToGen converts a store.ApplyRun into its wire shape. Warnings
// is always a non-nil slice so a run that dropped nothing serializes as
// [] rather than null -- the page renders "no conflicts" from an empty
// array and would otherwise have to special-case null too.
func applyRunToGen(run store.ApplyRun) gen.ApplyRun {
	warnings := make([]gen.ApplyRunWarning, 0, len(run.Warnings))
	for _, w := range run.Warnings {
		blockName := w.BlockName
		occurrenceStart := w.OccurrenceStart
		blocking := w.BlockingBlockName
		channelID := w.ChannelID
		duration := w.DurationMinutes
		warnings = append(warnings, gen.ApplyRunWarning{
			BlockName:         &blockName,
			OccurrenceStart:   &occurrenceStart,
			BlockingBlockName: &blocking,
			ChannelId:         &channelID,
			DurationMinutes:   &duration,
		})
	}

	scope := run.Scope
	days := run.Days
	channelCount := run.ChannelCount
	slotCount := run.SlotCount
	errText := run.Error

	return gen.ApplyRun{
		Id:           run.ID,
		StartedAt:    run.StartedAt,
		FinishedAt:   run.FinishedAt,
		Source:       gen.ApplyRunSource(run.Source),
		Scope:        &scope,
		Days:         &days,
		Status:       gen.ApplyRunStatus(run.Status),
		ChannelCount: &channelCount,
		SlotCount:    &slotCount,
		Error:        &errText,
		Warnings:     &warnings,
	}
}
