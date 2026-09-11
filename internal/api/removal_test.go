package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

func decodeRemovalReport(t *testing.T, w *httptest.ResponseRecorder) gen.RemovalReport {
	t.Helper()
	var report gen.RemovalReport
	require.NoError(t, json.NewDecoder(w.Body).Decode(&report), "body: %s", w.Body.String())
	return report
}

func seedTrackedShow(t *testing.T, s *store.Store, showTitle string, airings int) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.UpdateSeriesState(ctx, &scheduler.SeriesState{
		ShowTitle: showTitle, CurrentSeason: 1, CurrentEpisode: 1,
	}))

	at := time.Now().Add(-48 * time.Hour)
	rows := make([]scheduler.ScheduleHistoryEntry, 0, airings)
	for i := range airings {
		stamp := at.Add(time.Duration(i) * time.Hour)
		rows = append(rows, scheduler.ScheduleHistoryEntry{
			ProgramID: fmt.Sprintf("p-%d", i), ChannelID: "ch1", BlockName: "Night",
			ScheduledAt: stamp, OccurrenceStart: stamp,
			Title: "Episode", ShowTitle: showTitle, Type: "episode",
		})
	}
	require.NoError(t, s.RecordScheduleHistory(ctx, rows))
}

func removalPath(showTitle string) string {
	return "/state/series/" + url.PathEscape(showTitle)
}

// ---- per-show removal ------------------------------------------------------

func TestRemoveSeriesState_DryRunCountsWithoutRemoving(t *testing.T) {
	h, s := newTestServerWithStore(t)
	seedTrackedShow(t, s, "Bloody Mary", 3)

	w := doRequest(t, h, http.MethodDelete, removalPath("Bloody Mary")+"?dry_run=true", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	report := decodeRemovalReport(t, w)
	assert.Equal(t, 3, report.Airings)
	require.NotNil(t, report.DryRun)
	assert.True(t, *report.DryRun)

	left, err := s.ListScheduleHistory(t.Context(), time.Time{})
	require.NoError(t, err)
	assert.Len(t, left, 3, "a dry run removes nothing")
}

func TestRemoveSeriesState_RemovesAndReportsPerTable(t *testing.T) {
	h, s := newTestServerWithStore(t)
	seedTrackedShow(t, s, "Bloody Mary", 2)

	w := doRequest(t, h, http.MethodDelete, removalPath("Bloody Mary"), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	report := decodeRemovalReport(t, w)
	assert.Equal(t, 2, report.Airings)
	require.NotNil(t, report.SeriesStates)
	assert.Equal(t, 1, *report.SeriesStates)

	_, err := s.GetPersistedSeriesState(t.Context(), "Bloody Mary")
	require.Error(t, err, "the cursor is gone")
}

// The refusal is the interesting path: the next apply would re-add the
// show from the block spec and reset the cursor the removal just deleted.
func TestRemoveSeriesState_409WhileABlockStillListsTheShow(t *testing.T) {
	h, s := newTestServerWithStore(t)
	seedTrackedShow(t, s, "Bloody Mary", 1)

	require.NoError(t, s.CreateBlock(t.Context(), &store.BlockRecord{
		ID: "b1", Name: "Anime Night", Enabled: true,
		Spec: scheduler.Block{
			Name: "Anime Night", Type: scheduler.BlockTypeSeries, Cron: "0 22 * * *",
			Duration: 60, ChannelID: "ch1",
			Series: []scheduler.SeriesConfig{{ShowTitle: "Bloody Mary"}},
		},
	}))

	w := doRequest(t, h, http.MethodDelete, removalPath("Bloody Mary"), nil)
	require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "Anime Night", "the refusal names the block to edit first")

	// And the dry run refuses identically, so the operator cannot be
	// promised a removal the real call would reject.
	dry := doRequest(t, h, http.MethodDelete, removalPath("Bloody Mary")+"?dry_run=true", nil)
	assert.Equal(t, http.StatusConflict, dry.Code, dry.Body.String())
}

func TestRemoveSeriesState_UnknownTitleIs404(t *testing.T) {
	h, _ := newTestServerWithStore(t)
	w := doRequest(t, h, http.MethodDelete, removalPath("Never Tracked"), nil)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

// ---- range deletion --------------------------------------------------------

func TestDeleteHistoryRange_RequiresExactlyOneWindowForm(t *testing.T) {
	h, _ := newTestServerWithStore(t)

	for _, q := range []string{
		"", // neither
		"?before=2026-05-01T00:00:00Z&from=2026-05-01T00:00:00Z", // both
	} {
		w := doRequest(t, h, http.MethodDelete, "/history"+q, nil)
		assert.Equalf(t, http.StatusBadRequest, w.Code, "query %q: %s", q, w.Body.String())
	}
}

func TestDeleteHistoryRange_RefusesABackwardSpan(t *testing.T) {
	h, _ := newTestServerWithStore(t)
	w := doRequest(t, h, http.MethodDelete,
		"/history?from=2026-05-09T00:00:00Z&to=2026-05-01T00:00:00Z", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestDeleteHistoryRange_DryRunThenDeleteAgree(t *testing.T) {
	h, s := newTestServerWithStore(t)
	seedTrackedShow(t, s, "Bloody Mary", 3)

	cutoff := time.Now().Format(time.RFC3339)
	path := "/history?before=" + url.QueryEscape(cutoff)

	dry := doRequest(t, h, http.MethodDelete, path+"&dry_run=true", nil)
	require.Equal(t, http.StatusOK, dry.Code, dry.Body.String())
	preview := decodeRemovalReport(t, dry)
	assert.Equal(t, 3, preview.Airings)

	left, err := s.ListScheduleHistory(t.Context(), time.Time{})
	require.NoError(t, err)
	require.Len(t, left, 3, "a dry run deletes nothing")

	real := doRequest(t, h, http.MethodDelete, path, nil)
	require.Equal(t, http.StatusOK, real.Code, real.Body.String())
	actual := decodeRemovalReport(t, real)

	// The operator confirms against the preview.
	assert.Equal(t, preview.Airings, actual.Airings)
	assert.Equal(t, preview.EmptiedSlots, actual.EmptiedSlots)
}

func TestDeleteHistoryRange_NarrowsByShow(t *testing.T) {
	h, s := newTestServerWithStore(t)
	seedTrackedShow(t, s, "Bloody Mary", 2)
	seedTrackedShow(t, s, "Other Show", 2)

	cutoff := url.QueryEscape(time.Now().Format(time.RFC3339))
	w := doRequest(t, h, http.MethodDelete, "/history?before="+cutoff+"&show_title=Bloody+Mary", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 2, decodeRemovalReport(t, w).Airings)

	left, err := s.ListScheduleHistory(t.Context(), time.Time{})
	require.NoError(t, err)
	for _, e := range left {
		assert.NotEqual(t, "Bloody Mary", e.ShowTitle)
	}
}

// ---- storage ---------------------------------------------------------------

func TestGetStorage_ReportsRowsAndBounds(t *testing.T) {
	h, s := newTestServerWithStore(t)
	seedTrackedShow(t, s, "Bloody Mary", 2)

	w := doRequest(t, h, http.MethodGet, "/storage", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var report gen.StorageReport
	require.NoError(t, json.NewDecoder(w.Body).Decode(&report))
	assert.Equal(t, 2, report.Airings)
	assert.Equal(t, 1, report.SeriesStates)
	assert.NotNil(t, report.OldestAiring, "the bounds prefill a range form")
	assert.NotNil(t, report.NewestAiring)
}
