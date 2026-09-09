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
	"github.com/christopherime/schedularr/internal/store"
)

func decodeApplyRunList(t *testing.T, w *httptest.ResponseRecorder) []gen.ApplyRun {
	t.Helper()
	var list []gen.ApplyRun
	require.NoError(t, json.NewDecoder(w.Body).Decode(&list), "body: %s", w.Body.String())
	return list
}

func TestListApplyRuns_ReturnsRunsWithWarnings(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	started := time.Now().Add(-time.Minute)
	run := store.ApplyRun{
		ID: "r1", StartedAt: started, Source: store.ApplySourceUI,
		Days: 7, Status: store.ApplyStatusRunning,
	}
	require.NoError(t, s.StartApplyRun(ctx, run))

	finished := started.Add(time.Second)
	run.FinishedAt = &finished
	run.Status = store.ApplyStatusOK
	run.ChannelCount, run.SlotCount = 1, 4
	run.Warnings = []store.ApplyRunWarning{{
		BlockName: "Late Movie", OccurrenceStart: started, BlockingBlockName: "News",
		ChannelID: "ch-1", DurationMinutes: 120,
	}}
	require.NoError(t, s.FinishApplyRun(ctx, run))

	w := doRequest(t, h, http.MethodGet, "/applies?days=7", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	list := decodeApplyRunList(t, w)
	require.Len(t, list, 1)

	got := list[0]
	assert.Equal(t, "r1", got.Id)
	assert.Equal(t, gen.ApplyRunStatus("ok"), got.Status)
	assert.Equal(t, gen.ApplyRunSource("ui"), got.Source)
	require.NotNil(t, got.SlotCount)
	assert.Equal(t, 4, *got.SlotCount)
	require.NotNil(t, got.FinishedAt)

	require.NotNil(t, got.Warnings)
	require.Len(t, *got.Warnings, 1)
	warning := (*got.Warnings)[0]
	require.NotNil(t, warning.BlockingBlockName)
	assert.Equal(t, "News", *warning.BlockingBlockName)
	require.NotNil(t, warning.DurationMinutes)
	assert.Equal(t, 120, *warning.DurationMinutes)
	require.NotNil(t, warning.ChannelId)
	assert.Equal(t, "ch-1", *warning.ChannelId)
}

// TestListApplyRuns_WarningsAreAlwaysAnArray pins the wire promise the
// page depends on: a run that dropped nothing serializes warnings as [],
// never null, so the client renders "no conflicts" without a null branch.
func TestListApplyRuns_WarningsAreAlwaysAnArray(t *testing.T) {
	h, s := newTestServerWithStore(t)
	ctx := t.Context()

	require.NoError(t, s.StartApplyRun(ctx, store.ApplyRun{
		ID: "quiet", StartedAt: time.Now().Add(-time.Minute),
		Source: store.ApplySourceCron, Status: store.ApplyStatusOK,
	}))

	w := doRequest(t, h, http.MethodGet, "/applies", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"warnings":[]`)
}

// TestListApplyRuns_InFlightRunIsVisible: the row is written before the
// apply touches Tunarr, so a run still in flight -- or one whose process
// died mid-apply -- is reported rather than hidden.
func TestListApplyRuns_InFlightRunIsVisible(t *testing.T) {
	h, s := newTestServerWithStore(t)

	require.NoError(t, s.StartApplyRun(t.Context(), store.ApplyRun{
		ID: "inflight", StartedAt: time.Now().Add(-time.Minute),
		Source: store.ApplySourceCLI, Status: store.ApplyStatusRunning,
	}))

	w := doRequest(t, h, http.MethodGet, "/applies", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	list := decodeApplyRunList(t, w)
	require.Len(t, list, 1)
	assert.Equal(t, gen.ApplyRunStatus("running"), list[0].Status)
	assert.Nil(t, list[0].FinishedAt, "an in-flight run has no finish stamp")
}

func TestListApplyRuns_RejectsOutOfRangeParams(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	for _, query := range []string{"/applies?days=200", "/applies?days=0", "/applies?limit=0", "/applies?limit=501"} {
		w := doRequest(t, h, http.MethodGet, query, nil)
		assert.Equalf(t, http.StatusBadRequest, w.Code, "%s should be rejected: %s", query, w.Body.String())
	}
}
