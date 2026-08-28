package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/scheduler"
)

func decodeHistoryList(t *testing.T, w *httptest.ResponseRecorder) []gen.HistoryEntry {
	t.Helper()
	var list []gen.HistoryEntry
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list), "body: %s", w.Body.String())
	return list
}

func TestGetHistory_DefaultDays_FiltersToLast7DaysDesc(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	now := time.Now()
	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		// Older than the default 7-day window -- must be excluded.
		{ProgramID: "old-1", ChannelID: "ch1", BlockName: "block-a", ScheduledAt: now.Add(-10 * 24 * time.Hour)},
		// Within the window -- must be included, most recent first.
		{ProgramID: "new-1", ChannelID: "ch1", BlockName: "block-b", ScheduledAt: now.Add(-1 * time.Hour)},
		{ProgramID: "new-2", ChannelID: "ch2", BlockName: "block-b", ScheduledAt: now.Add(-2 * time.Hour)},
	}))

	w := doRequest(t, h, http.MethodGet, "/history", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	list := decodeHistoryList(t, w)
	require.Len(t, list, 2)
	require.NotNil(t, list[0].ProgramId)
	require.NotNil(t, list[1].ProgramId)
	assert.Equal(t, "new-1", *list[0].ProgramId, "most recently scheduled entry first")
	assert.Equal(t, "new-2", *list[1].ProgramId)
}

func TestGetHistory_DaysParam_NarrowsWindow(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	now := time.Now()
	require.NoError(t, s.RecordScheduleHistory(ctx, []scheduler.ScheduleHistoryEntry{
		{ProgramID: "in-3d", ChannelID: "ch1", BlockName: "block-a", ScheduledAt: now.Add(-2 * 24 * time.Hour)},
		{ProgramID: "out-3d", ChannelID: "ch1", BlockName: "block-a", ScheduledAt: now.Add(-5 * 24 * time.Hour)},
	}))

	w := doRequest(t, h, http.MethodGet, "/history?days=3", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	list := decodeHistoryList(t, w)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].ProgramId)
	assert.Equal(t, "in-3d", *list[0].ProgramId)
}

func TestGetHistory_Empty(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	w := doRequest(t, h, http.MethodGet, "/history", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	list := decodeHistoryList(t, w)
	assert.Empty(t, list)
}

func TestGetHistory_DaysTooHigh_BadRequest(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	w := doRequest(t, h, http.MethodGet, "/history?days=200", nil)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
}

func TestGetHistory_DaysZero_BadRequest(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	w := doRequest(t, h, http.MethodGet, "/history?days=0", nil)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
}

func TestGetHistory_DaysNegative_BadRequest(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	w := doRequest(t, h, http.MethodGet, "/history?days=-1", nil)
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestGetHistory_InternalErrorDoesNotLeakDetail(t *testing.T) {
	h := newClosedStoreServer(t)

	w := doRequest(t, h, http.MethodGet, "/history", nil)
	assertGenericInternalErrorProblem(t, w)
}
