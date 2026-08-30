package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/service"
)

// Schedule day-window bounds shared by POST /generate, POST /apply (both
// take GenerateRequest, whose days field the OpenAPI schema marks default
// 7, minimum 1, maximum 30) and GET /schedule?days=N (same bounds, in its
// query parameter). As with GetHistory's days handling
// (internal/api/history.go), oapi-codegen's generated bindings do not
// enforce the schema's default/minimum/maximum, so both are applied here.
const (
	defaultScheduleDays = 7
	minScheduleDays     = 1
	maxScheduleDays     = 30
)

// GenerateSchedule implements gen.ServerInterface. It always runs a dry
// run (service.Options.Apply: false) regardless of the request body --
// only POST /apply mutates anything; this route exists purely to preview
// a plan.
func (h *Handlers) GenerateSchedule(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeGenerateRequest(w, r)
	if !ok {
		return
	}
	h.runSchedule(w, r, scheduleRequest{days: body.Days, channelID: body.ChannelId, apply: false, op: "generate_schedule"})
}

// ApplySchedule implements gen.ServerInterface. Unlike GenerateSchedule, it
// runs with service.Options.Apply: true, so a successful response means
// Runner.Run pushed the plan to Tunarr (UpdateSchedule per channel) and
// committed the engine's pending state.
func (h *Handlers) ApplySchedule(w http.ResponseWriter, r *http.Request) {
	body, ok := h.decodeGenerateRequest(w, r)
	if !ok {
		return
	}
	h.runSchedule(w, r, scheduleRequest{days: body.Days, channelID: body.ChannelId, apply: true, op: "apply_schedule"})
}

// GetSchedule implements gen.ServerInterface. It always dry-runs (there is
// no request body on a GET, and no apply concept for this route). channel_id
// scopes planning exactly like GenerateRequest.channel_id does on the POST
// routes -- the web guide's SCOPE control rides on this parameter.
func (h *Handlers) GetSchedule(w http.ResponseWriter, r *http.Request, params gen.GetScheduleParams) {
	h.runSchedule(w, r, scheduleRequest{days: params.Days, channelID: params.ChannelId, op: "get_schedule"})
}

// decodeGenerateRequest decodes a GenerateRequest body for GenerateSchedule
// and ApplySchedule. Unlike CreateBlock/UpdateBlock's BlockWrite (whose
// OpenAPI requestBody is required and whose fields are required too), the
// OpenAPI schema does not mark GenerateRequest's body required and every
// one of its fields is optional -- so a request with no body at all (the
// decode failing with io.EOF) is valid input, not a 400: it just means
// "use every default."
func (h *Handlers) decodeGenerateRequest(w http.ResponseWriter, r *http.Request) (gen.GenerateRequest, bool) {
	var body gen.GenerateRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return body, true
		}
		WriteProblem(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return gen.GenerateRequest{}, false
	}
	return body, true
}

// scheduleRequest bundles runSchedule's inputs: GenerateSchedule and
// ApplySchedule build one from a decoded JSON body, GetSchedule from its
// query parameters. op identifies the calling handler for
// writeScheduleRunnerError's server-side log line (e.g.
// "generate_schedule").
type scheduleRequest struct {
	days      *int
	channelID *string
	apply     bool
	op        string
}

// runSchedule validates days, resolves the optional channelID, calls
// Deps.Sched.Run, and writes the resulting plan -- the shared body behind
// all three schedule endpoints, which differ only in where their
// parameters come from (query string vs. JSON body) and whether apply is
// set.
func (h *Handlers) runSchedule(w http.ResponseWriter, r *http.Request, req scheduleRequest) {
	days := defaultScheduleDays
	if req.days != nil {
		days = *req.days
	}
	if days < minScheduleDays || days > maxScheduleDays {
		WriteProblem(w, r, http.StatusBadRequest, "invalid days parameter",
			fmt.Sprintf("days must be between %d and %d, got %d", minScheduleDays, maxScheduleDays, days))
		return
	}

	channelID := ""
	if req.channelID != nil {
		channelID = *req.channelID
	}

	result, err := h.d.Sched.Run(r.Context(), service.Options{Days: days, ChannelID: channelID, Apply: req.apply})
	if err != nil {
		h.writeScheduleRunnerError(w, r, req.op, err)
		return
	}

	writeJSON(w, http.StatusOK, planResultToGen(result))
}

// writeScheduleRunnerError logs err server-side and writes a 502
// problem+json response with a short, fixed detail. Unlike ListChannels'
// Tunarr-connectivity errors (safe to echo verbatim -- see tunarr.go), a
// Runner.Run failure can wrap a raw store/driver error (e.g.
// Engine.Commit's error path during apply), so -- matching
// logAndWriteInternalError's information-leak convention -- the real error
// only ever reaches the server log, never the response body. op identifies
// which handler code path failed (e.g. "generate_schedule").
func (h *Handlers) writeScheduleRunnerError(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.d.Logger.Error("schedule generation failed",
		"op", op,
		"request_id", RequestIDFromContext(r.Context()),
		"error", err,
	)
	WriteProblem(w, r, http.StatusBadGateway, "schedule generation failed", "unable to generate or apply the schedule")
}

// planResultToGen converts a service.Result (the domain representation) to
// a gen.PlanResult (the API wire shape). Warnings is omitted entirely
// (nil, not an empty array) when result.Warnings is empty, matching
// OpenAPI's `warnings` being optional -- a caller with nothing to report
// sees no key at all, not a distracting `"warnings": []` on every
// response.
func planResultToGen(result *service.Result) gen.PlanResult {
	channels := make(map[string][]gen.ScheduledSlot, len(result.Channels))
	for channelID, slots := range result.Channels {
		list := make([]gen.ScheduledSlot, 0, len(slots))
		for _, slot := range slots {
			list = append(list, slotToGen(slot))
		}
		channels[channelID] = list
	}

	plan := gen.PlanResult{Applied: result.Applied, Channels: channels}
	if len(result.Warnings) > 0 {
		warnings := make([]gen.Warning, 0, len(result.Warnings))
		for _, w := range result.Warnings {
			warnings = append(warnings, warningToGen(w))
		}
		plan.Warnings = &warnings
	}
	return plan
}

// warningToGen converts a scheduler.Warning into a gen.Warning.
func warningToGen(w scheduler.Warning) gen.Warning {
	blockName := w.BlockName
	occurrenceStart := w.OccurrenceStart
	blockingBlockName := w.BlockingBlockName
	return gen.Warning{
		BlockName:         &blockName,
		OccurrenceStart:   &occurrenceStart,
		BlockingBlockName: &blockingBlockName,
	}
}

// slotToGen converts a scheduler.ScheduledSlot into a gen.ScheduledSlot.
// Block reuses specToGen (blocks.go), the same scheduler.Block ->
// gen.BlockSpec conversion ListBlocks/GetBlock/... use. Programs is the
// typed ScheduledProgram projection (a hard swap from the old
// additionalProperties passthrough, per the no-legacy policy): each
// program's own start_time is computed here as the slot's start plus the
// cumulative durations of everything before it in the lineup -- the same
// wall-clock math Tunarr applies when it plays the lineup back to back.
func slotToGen(slot scheduler.ScheduledSlot) gen.ScheduledSlot {
	start := slot.StartTime
	end := slot.EndTime
	block := specToGen(slot.Block)

	programs := make([]gen.ScheduledProgram, 0, len(slot.Programs))
	cursor := slot.StartTime
	for _, p := range slot.Programs {
		programs = append(programs, programToGen(p, cursor))
		cursor = cursor.Add(time.Duration(p.GetDurationMs()) * time.Millisecond)
	}

	return gen.ScheduledSlot{
		StartTime: &start,
		EndTime:   &end,
		Block:     &block,
		Programs:  &programs,
	}
}

// programToGen projects one tunarr.Program onto the wire's typed
// ScheduledProgram at its computed air time. Optional fields are omitted
// (nil) rather than sent as zero values: a flex placeholder has no type
// worth naming beyond what Tunarr set, and season/episode 0 means "not an
// episode", not S00E00.
func programToGen(p tunarr.Program, startTime time.Time) gen.ScheduledProgram {
	out := gen.ScheduledProgram{
		Title:      p.Title,
		DurationMs: p.GetDurationMs(),
		StartTime:  startTime,
	}
	if p.Type != "" {
		t := p.Type
		out.Type = &t
	}
	if p.SeasonNumber > 0 {
		s := p.SeasonNumber
		out.Season = &s
	}
	if p.EpisodeNumber > 0 {
		e := p.EpisodeNumber
		out.Episode = &e
	}
	return out
}
