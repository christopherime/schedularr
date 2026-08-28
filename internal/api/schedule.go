package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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
// no request body on a GET, and no apply concept for this route).
func (h *Handlers) GetSchedule(w http.ResponseWriter, r *http.Request, params gen.GetScheduleParams) {
	h.runSchedule(w, r, scheduleRequest{days: params.Days, op: "get_schedule"})
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
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
// a gen.PlanResult (the API wire shape).
func planResultToGen(result *service.Result) gen.PlanResult {
	channels := make(map[string][]gen.ScheduledSlot, len(result.Channels))
	for channelID, slots := range result.Channels {
		list := make([]gen.ScheduledSlot, 0, len(slots))
		for _, slot := range slots {
			list = append(list, slotToGen(slot))
		}
		channels[channelID] = list
	}
	return gen.PlanResult{Applied: result.Applied, Channels: channels}
}

// slotToGen converts a scheduler.ScheduledSlot into a gen.ScheduledSlot.
// Block reuses specToGen (blocks.go), the same scheduler.Block ->
// gen.BlockSpec conversion ListBlocks/GetBlock/... use. Programs is a
// `additionalProperties: true` object array in the OpenAPI schema (see
// api/openapi.yaml), so each tunarr.Program round-trips through JSON into
// a map[string]interface{} rather than a typed struct.
func slotToGen(slot scheduler.ScheduledSlot) gen.ScheduledSlot {
	start := slot.StartTime
	end := slot.EndTime
	block := specToGen(slot.Block)

	programs := make([]map[string]interface{}, 0, len(slot.Programs))
	for _, p := range slot.Programs {
		programs = append(programs, programToGen(p))
	}

	return gen.ScheduledSlot{
		StartTime: &start,
		EndTime:   &end,
		Block:     &block,
		Programs:  &programs,
	}
}

// programToGen converts a tunarr.Program to the loosely-typed map the
// PlanResult.channels[].programs wire schema expects. The round trip
// through encoding/json cannot fail for a tunarr.Program (every field is a
// JSON-marshalable primitive, slice, or pointer to one) -- a marshal error
// here would mean the tunarr.Program type itself became non-serializable,
// which every other Tunarr response path in this codebase already assumes
// can't happen, so an error here degrades to an empty map rather than
// failing the whole response.
func programToGen(p tunarr.Program) map[string]interface{} {
	b, err := json.Marshal(p)
	if err != nil {
		return map[string]interface{}{}
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]interface{}{}
	}
	return m
}
