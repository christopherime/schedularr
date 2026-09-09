package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/scheduler"
)

// History window bounds for GET /history's days query parameter. These
// mirror the OpenAPI schema (api/openapi.yaml: default 7, minimum 1,
// maximum 90), which oapi-codegen's chi-server generator does not enforce
// at the binding layer -- see the GetHistory doc comment below.
const (
	defaultHistoryDays = 7
	minHistoryDays     = 1
	maxHistoryDays     = 90
)

// GetHistory implements gen.ServerInterface.
//
// gen.GetHistoryParams.Days is populated straight from the raw query string
// with no validation and no default substitution: the generated
// ServerInterfaceWrapper.GetHistory (internal/api/gen/server.gen.go) only
// calls runtime.BindQueryParameterWithOptions with Type: "integer" -- it
// does not apply the OpenAPI schema's default/minimum/maximum for the days
// parameter. So a nil Days here means "the client omitted it", not "0" or
// "7", and an out-of-range value reaches this handler unrejected; both are
// handled here rather than by generated code.
func (h *Handlers) GetHistory(w http.ResponseWriter, r *http.Request, params gen.GetHistoryParams) {
	days := defaultHistoryDays
	if params.Days != nil {
		days = *params.Days
	}
	if days < minHistoryDays || days > maxHistoryDays {
		WriteProblem(w, r, http.StatusBadRequest, "invalid days parameter",
			fmt.Sprintf("days must be between %d and %d, got %d", minHistoryDays, maxHistoryDays, days))
		return
	}

	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	entries, err := h.d.Store.ListScheduleHistory(r.Context(), since)
	if err != nil {
		h.logAndWriteInternalError(w, r, "get_history", err)
		return
	}

	list := make([]gen.HistoryEntry, 0, len(entries))
	for _, e := range entries {
		list = append(list, historyEntryToGen(e))
	}
	writeJSON(w, http.StatusOK, list)
}

// historyEntryToGen converts a scheduler.ScheduleHistoryEntry (the persisted
// domain representation) into a gen.HistoryEntry (the API wire shape). Every
// wire field is a pointer (OpenAPI marks them optional) but always populated
// here since this only ever converts an already-persisted row.
//
// Since v0.5.7 the enrichment the schedule_history table has stored all
// along -- title, type, duration_ms, occurrence_start, sequence -- is
// exposed too, along with run_id: the history page shows program names
// and the apply that put them on air, not UUIDs.
func historyEntryToGen(e scheduler.ScheduleHistoryEntry) gen.HistoryEntry {
	programID := e.ProgramID
	channelID := e.ChannelID
	blockName := e.BlockName
	scheduledAt := e.ScheduledAt
	occurrenceStart := e.OccurrenceStart
	sequence := e.Sequence
	durationMs := e.DurationMs
	title := e.Title
	kind := e.Type
	runID := e.RunID

	return gen.HistoryEntry{
		ProgramId:       &programID,
		ChannelId:       &channelID,
		BlockName:       &blockName,
		ScheduledAt:     &scheduledAt,
		OccurrenceStart: &occurrenceStart,
		Sequence:        &sequence,
		DurationMs:      &durationMs,
		Title:           &title,
		Type:            &kind,
		RunId:           &runID,
	}
}
