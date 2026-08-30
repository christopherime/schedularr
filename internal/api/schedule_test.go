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

// TestGetSchedule_ChannelIDQueryParam covers the v0.5.1 contract addition:
// GET /schedule?channel_id=... scopes planning exactly like
// GenerateRequest.channel_id does on POST /generate and /apply.
func TestGetSchedule_ChannelIDQueryParam(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodGet, "/schedule?days=2&channel_id=chan-7", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, service.Options{Days: 2, ChannelID: "chan-7", Apply: false}, fake.lastOpts)
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
	assert.Equal(t, "Movie One", (*slots[0].Programs)[0].Title)
}

// TestGenerateSchedule_TypedProgramsWithCumulativeStartTimes pins the
// typed ScheduledProgram projection (the v0.5.1 hard swap): every wire
// field populated from the internal tunarr.Program, per-program start_time
// computed cumulatively from the slot start, and the optional fields
// (type/season/episode) omitted -- not zero-filled -- when the source
// program has nothing to say.
func TestGenerateSchedule_TypedProgramsWithCumulativeStartTimes(t *testing.T) {
	start := time.Date(2026, 8, 28, 21, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	fake := &fakeScheduleRunner{result: &service.Result{
		Applied: false,
		Channels: map[string][]scheduler.ScheduledSlot{
			"channel-1": {
				{
					StartTime: start,
					EndTime:   end,
					Block:     scheduler.Block{Name: "Horror Night", Cron: "0 21 * * *", Duration: 120, ChannelID: "channel-1"},
					Programs: []tunarr.Program{
						{Title: "Pilot", Duration: 1_800_000, Type: "episode", SeasonNumber: 1, EpisodeNumber: 1},
						{Title: "Wendigo", Duration: 2_700_000, Type: "episode", SeasonNumber: 1, EpisodeNumber: 2},
						{Title: "Flex", Duration: 600_000},
					},
				},
			},
		},
	}}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodGet, "/schedule", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got gen.PlanResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.Contains(t, got.Channels, "channel-1")
	require.Len(t, got.Channels["channel-1"], 1)
	programs := *got.Channels["channel-1"][0].Programs
	require.Len(t, programs, 3)

	// Program 1: full episode metadata, starts at the slot start.
	assert.Equal(t, "Pilot", programs[0].Title)
	assert.EqualValues(t, 1_800_000, programs[0].DurationMs)
	require.NotNil(t, programs[0].Type)
	assert.Equal(t, "episode", *programs[0].Type)
	require.NotNil(t, programs[0].Season)
	assert.Equal(t, 1, *programs[0].Season)
	require.NotNil(t, programs[0].Episode)
	assert.Equal(t, 1, *programs[0].Episode)
	assert.True(t, programs[0].StartTime.Equal(start), "first program starts at the slot start")

	// Program 2: starts after program 1's 30 minutes.
	assert.True(t, programs[1].StartTime.Equal(start.Add(30*time.Minute)),
		"second program's start_time must be slot start + first program's duration")
	require.NotNil(t, programs[1].Episode)
	assert.Equal(t, 2, *programs[1].Episode)

	// Program 3: a flex placeholder -- cumulative math continues (30 + 45
	// minutes), and type/season/episode are omitted entirely.
	assert.True(t, programs[2].StartTime.Equal(start.Add(75*time.Minute)),
		"third program's start_time must accumulate both prior durations")
	assert.Nil(t, programs[2].Type)
	assert.Nil(t, programs[2].Season)
	assert.Nil(t, programs[2].Episode)
	body := w.Body.String()
	assert.NotContains(t, body, `"season":0`, "zero season must be omitted, not sent")
	assert.NotContains(t, body, `"type":""`, "empty type must be omitted, not sent")
}

// TestGenerateSchedule_MapsWarningsToPlanResult is the API-contract half
// of the "conflict resolution is invisible to callers" fix: a dropped
// occurrence used to be visible only in a server-side INFO log line; now
// it's on the response itself.
func TestGenerateSchedule_MapsWarningsToPlanResult(t *testing.T) {
	occStart := time.Date(2026, 8, 28, 6, 0, 0, 0, time.UTC)
	fake := &fakeScheduleRunner{result: &service.Result{
		Applied:  false,
		Channels: map[string][]scheduler.ScheduledSlot{},
		Warnings: []scheduler.Warning{
			{BlockName: "Low Priority Block", OccurrenceStart: occStart, BlockingBlockName: "High Priority Block"},
		},
	}}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/generate", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var got gen.PlanResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&got))
	require.NotNil(t, got.Warnings)
	require.Len(t, *got.Warnings, 1)

	warning := (*got.Warnings)[0]
	require.NotNil(t, warning.BlockName)
	assert.Equal(t, "Low Priority Block", *warning.BlockName)
	require.NotNil(t, warning.BlockingBlockName)
	assert.Equal(t, "High Priority Block", *warning.BlockingBlockName)
	require.NotNil(t, warning.OccurrenceStart)
	assert.True(t, warning.OccurrenceStart.Equal(occStart))
}

// TestGenerateSchedule_NoWarnings_OmitsWarningsField asserts the other
// half of planResultToGen's contract: a plan with nothing to warn about
// must omit the "warnings" key entirely (nil), not send a distracting
// empty array on every ordinary response.
func TestGenerateSchedule_NoWarnings_OmitsWarningsField(t *testing.T) {
	fake := &fakeScheduleRunner{}
	h := newTestServerWithSched(t, fake)

	w := doRequest(t, h, http.MethodPost, "/generate", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.NotContains(t, w.Body.String(), `"warnings"`,
		"a plan with no warnings must omit the field entirely, not send an empty array")
}
