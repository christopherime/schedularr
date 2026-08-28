package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/service"
	"github.com/christopherime/schedularr/internal/store"
)

// fakeScheduleRunner is a test double for service.ScheduleRunner. It
// records the Options the handler passed (so tests can assert the handler
// translated request days/channel_id/route correctly) and returns either a
// canned result or a canned error.
type fakeScheduleRunner struct {
	lastOpts service.Options
	called   bool

	result *service.Result
	err    error
}

func (f *fakeScheduleRunner) Run(_ context.Context, o service.Options) (*service.Result, error) {
	f.called = true
	f.lastOpts = o

	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &service.Result{Applied: o.Apply, Channels: map[string][]scheduler.ScheduledSlot{}}, nil
}

// newTestServerWithSched builds the same kind of router newTestServer does
// (a fresh temp-dir sqlite store, no auth middleware), but with Deps.Sched
// set to sched -- a fake in every test in this file, since Runner.Run
// itself is exercised against a real store/Tunarr fake in
// internal/service/schedule_test.go, not here.
func newTestServerWithSched(t *testing.T, sched service.ScheduleRunner) http.Handler {
	t.Helper()
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err, "failed to create test store")
	t.Cleanup(func() { _ = s.Close() })

	h := NewHandlers(Deps{Store: s, Logger: slog.Default(), Version: "test", Sched: sched})
	return gen.HandlerFromMux(h, chi.NewRouter())
}

func intPtr(i int) *int       { return &i }
func strPtr(s string) *string { return &s }

func TestGenerateSchedule_AlwaysDryRunRegardlessOfBody(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/generate", gen.GenerateRequest{
		Days:      intPtr(5),
		ChannelId: strPtr("chan-1"),
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))

	require.True(t, fake.called)
	assert.Equal(t, service.Options{Days: 5, ChannelID: "chan-1", Apply: false}, fake.lastOpts)

	var got gen.PlanResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.False(t, got.Applied, "GenerateSchedule must never report applied:true")
}

func TestGenerateSchedule_EmptyBodyUsesDefaults(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/generate", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, service.Options{Days: defaultScheduleDays, ChannelID: "", Apply: false}, fake.lastOpts)
}

func TestApplySchedule_RunsWithApplyTrue(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/apply", gen.GenerateRequest{Days: intPtr(2)})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	require.True(t, fake.called)
	assert.Equal(t, service.Options{Days: 2, ChannelID: "", Apply: true}, fake.lastOpts)

	var got gen.PlanResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	assert.True(t, got.Applied)
}

func TestApplySchedule_ChannelIDPassedThrough(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/apply", gen.GenerateRequest{ChannelId: strPtr("chan-9")})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, "chan-9", fake.lastOpts.ChannelID)
	assert.True(t, fake.lastOpts.Apply)
}

func TestGetSchedule_DaysQueryParam(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodGet, "/schedule?days=3", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, service.Options{Days: 3, ChannelID: "", Apply: false}, fake.lastOpts)
}

func TestGetSchedule_DefaultDays(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodGet, "/schedule", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, defaultScheduleDays, fake.lastOpts.Days)
	assert.False(t, fake.lastOpts.Apply)
}

func TestGetSchedule_DaysOutOfRange_BadRequest(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	for _, days := range []string{"0", "-1", "31", "200"} {
		w := doRequest(t, h, http.MethodGet, "/schedule?days="+days, nil)
		require.Equal(t, http.StatusBadRequest, w.Code, "days=%s: %s", days, w.Body.String())
		assert.False(t, fake.called, "days=%s: runner must not be called for an invalid days value", days)
	}
}

func TestGenerateSchedule_DaysOutOfRange_BadRequest(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/generate", gen.GenerateRequest{Days: intPtr(31)})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.False(t, fake.called)

	p := decodeProblem(t, w)
	assert.Equal(t, http.StatusBadRequest, p.Status)
}

func TestGenerateSchedule_InvalidJSONBody_BadRequest(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/generate", strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
	assert.False(t, fake.called)
}

func TestRunSchedule_RunnerError_502DoesNotLeakDetail(t *testing.T) {
	fake := &fakeScheduleRunner{err: errors.New("sql: database is closed (driver internals)")}
	h := newTestServerWithSched(t, fake)

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodPost, "/generate", nil},
		{http.MethodPost, "/apply", nil},
		{http.MethodGet, "/schedule", nil},
	} {
		w := doRequest(t, h, tc.method, tc.path, tc.body)
		require.Equal(t, http.StatusBadGateway, w.Code, "%s %s: %s", tc.method, tc.path, w.Body.String())
		require.Equal(t, "application/problem+json", w.Header().Get("Content-Type"))

		p := decodeProblem(t, w)
		assert.Equal(t, http.StatusBadGateway, p.Status)
		assert.Equal(t, "schedule generation failed", p.Title)
		lower := strings.ToLower(p.Detail)
		assert.NotContains(t, lower, "sql")
		assert.NotContains(t, lower, "database")
		assert.NotContains(t, lower, "driver")
	}
}

func TestGenerateSchedule_MapsResultToPlanResult(t *testing.T) {
	start := time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC)
	end := start.Add(60 * time.Minute)

	fake := &fakeScheduleRunner{result: &service.Result{
		Applied: false,
		Channels: map[string][]scheduler.ScheduledSlot{
			"channel-1": {
				{
					StartTime: start,
					EndTime:   end,
					Block: scheduler.Block{
						Name:      "Morning Movies",
						Cron:      "0 6 * * *",
						Duration:  60,
						ChannelID: "channel-1",
					},
					Programs: []tunarr.Program{
						{ID: "prog-1", Title: "Movie One", Duration: 1_800_000, Type: "movie"},
					},
				},
			},
		},
	}}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/generate", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got gen.PlanResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.Contains(t, got.Channels, "channel-1")
	slots := got.Channels["channel-1"]
	require.Len(t, slots, 1)

	require.NotNil(t, slots[0].Block)
	assert.Equal(t, "Morning Movies", slots[0].Block.Name)

	require.NotNil(t, slots[0].StartTime)
	assert.True(t, slots[0].StartTime.Equal(start))

	require.NotNil(t, slots[0].Programs)
	require.Len(t, *slots[0].Programs, 1)
	assert.Equal(t, "Movie One", (*slots[0].Programs)[0]["title"])
}
