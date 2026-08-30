package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/blockio"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// ListBlocks implements gen.ServerInterface.
func (h *Handlers) ListBlocks(w http.ResponseWriter, r *http.Request) {
	recs, err := h.d.Store.ListBlocks(r.Context())
	if err != nil {
		h.logAndWriteInternalError(w, r, "list_blocks", err)
		return
	}

	list := make(gen.BlockList, 0, len(recs))
	for _, rec := range recs {
		list = append(list, toGen(rec))
	}
	writeJSON(w, http.StatusOK, list)
}

// CreateBlock implements gen.ServerInterface.
func (h *Handlers) CreateBlock(w http.ResponseWriter, r *http.Request) {
	var body gen.BlockWrite
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if err := validateRequiredSpecFields(body.Spec); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "block validation failed", err.Error())
		return
	}

	spec := fromGen(body.Spec)
	if err := validateSeriesShowTitles(spec); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "block validation failed", err.Error())
		return
	}
	if err := blockio.ValidateBlocks([]scheduler.Block{spec}); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "block validation failed", err.Error())
		return
	}

	rec := &store.BlockRecord{
		ID:      uuid.NewString(),
		Name:    spec.Name,
		Enabled: blockEnabled(body.Enabled),
		Spec:    spec,
	}

	if err := h.d.Store.CreateBlock(r.Context(), rec); err != nil {
		h.writeBlockStoreError(w, r, "create_block", err)
		return
	}

	writeJSON(w, http.StatusCreated, toGen(*rec))
}

// GetBlock implements gen.ServerInterface.
func (h *Handlers) GetBlock(w http.ResponseWriter, r *http.Request, id string) {
	rec, err := h.d.Store.GetBlock(r.Context(), id)
	if err != nil {
		h.writeBlockStoreError(w, r, "get_block", err)
		return
	}
	writeJSON(w, http.StatusOK, toGen(*rec))
}

// UpdateBlock implements gen.ServerInterface.
//
// PUT replaces the block's spec, enabled flag, and (if body.Spec.Name
// differs from the stored name) its name: BlockWrite carries the full
// desired spec, including name, so a body whose spec.name doesn't match the
// existing record's name renames the block rather than being rejected.
// store.UpdateBlock enforces the same unique-name constraint CreateBlock
// does, so a rename that collides with another block's name surfaces as
// ErrConflict -> 409, same as a colliding create.
//
// After a successful update, every not-yet-FINISHED occurrence snapshot
// for this block ID is deleted (DeleteFutureOccurrenceSnapshots, with
// store.InvalidationCutoff's widened cutoff -- see its doc comment for
// why "not yet finished" and not just "not yet started") so the next
// apply re-derives those occurrences from the
// just-edited spec instead of silently keeping a snapshot/committed
// assignment captured under the OLD spec until it ages out of the
// schedule-generation window on its own -- see
// StateStore.DeleteFutureOccurrenceSnapshots' doc comment. This is a
// series-only mechanism (only series blocks have occurrence snapshots at
// all), but it's harmless -- a no-op deleting zero rows -- to call
// unconditionally for a filter block too, so this doesn't special-case on
// existing.Spec.Type. A failure at this step is logged and NOT surfaced
// as a response error: store.UpdateBlock has already committed by then,
// so the write genuinely succeeded, and returning 500 for it would be
// both wrong (the caller's PUT did what it asked) and unhelpful (there's
// no compensating action for the caller to take -- see logInternalError's
// doc comment).
func (h *Handlers) UpdateBlock(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := h.d.Store.GetBlock(r.Context(), id)
	if err != nil {
		h.writeBlockStoreError(w, r, "update_block_lookup", err)
		return
	}

	var body gen.BlockWrite
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if err := validateRequiredSpecFields(body.Spec); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "block validation failed", err.Error())
		return
	}

	spec := fromGen(body.Spec)
	if err := validateSeriesShowTitles(spec); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "block validation failed", err.Error())
		return
	}
	if err := blockio.ValidateBlocks([]scheduler.Block{spec}); err != nil {
		WriteProblem(w, r, http.StatusBadRequest, "block validation failed", err.Error())
		return
	}

	existing.Name = spec.Name
	existing.Enabled = blockEnabled(body.Enabled)
	existing.Spec = spec

	if err := h.d.Store.UpdateBlock(r.Context(), existing); err != nil {
		h.writeBlockStoreError(w, r, "update_block", err)
		return
	}

	if err := h.d.Store.DeleteFutureOccurrenceSnapshots(r.Context(), id, store.InvalidationCutoff(time.Now(), existing.Spec.Duration)); err != nil {
		h.logInternalError(r, "update_block_invalidate_snapshots", err)
	}

	writeJSON(w, http.StatusOK, toGen(*existing))
}

// DeleteBlock implements gen.ServerInterface.
//
// After a successful delete, every not-yet-FINISHED occurrence snapshot
// for this block ID is also deleted (DeleteFutureOccurrenceSnapshots,
// same widened cutoff as UpdateBlock) -- see UpdateBlock's doc comment
// for why a block mutation needs this (including why a failure here is
// logged and not surfaced as a response error). A deleted block can
// never generate a future occurrence again, so this is mostly tidiness
// rather than a live correctness bug the way UpdateBlock's case is, but
// it's the same one-line cleanup and keeps the invariant "a block ID with
// no corresponding block record has no leftover snapshots either" from
// silently drifting false over time (e.g. a block deleted and later
// re-created would otherwise never collide on ID -- IDs are fresh UUIDs
// -- but leaving orphaned rows around for a since-deleted ID serves no
// purpose). The block is looked up BEFORE deleting it -- its Duration is
// needed to compute the cutoff, and it's gone from the store once
// DeleteBlock succeeds.
func (h *Handlers) DeleteBlock(w http.ResponseWriter, r *http.Request, id string) {
	existing, err := h.d.Store.GetBlock(r.Context(), id)
	if err != nil {
		h.writeBlockStoreError(w, r, "delete_block_lookup", err)
		return
	}

	if err := h.d.Store.DeleteBlock(r.Context(), id); err != nil {
		h.writeBlockStoreError(w, r, "delete_block", err)
		return
	}

	if err := h.d.Store.DeleteFutureOccurrenceSnapshots(r.Context(), id, store.InvalidationCutoff(time.Now(), existing.Spec.Duration)); err != nil {
		h.logInternalError(r, "delete_block_invalidate_snapshots", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// blockEnabled applies BlockWrite.Enabled's OpenAPI default (true) when the
// caller omits the field.
func blockEnabled(enabled *bool) bool {
	if enabled == nil {
		return true
	}
	return *enabled
}

// writeBlockStoreError maps store errors to the appropriate problem+json
// response: ErrNotFound -> 404, ErrConflict -> 409, anything else -> 500.
// op identifies the calling handler operation (e.g. "create_block") for the
// server-side log line logAndWriteInternalError writes on the 500 path.
//
// ErrNotFound/ErrConflict details are user-facing (they name what the
// caller got wrong, e.g. a duplicate block name) and are not internal
// leaks, so they keep err.Error() as their detail. The default (unmapped)
// branch is different: err here is whatever the store returned, which can
// wrap a raw sqlite/sqlx driver error, so it goes through
// logAndWriteInternalError instead, matching the Recovery middleware's
// convention (internal/api/middleware/recovery.go) of a generic 500 detail
// with the real error logged server-side only.
func (h *Handlers) writeBlockStoreError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		WriteProblem(w, r, http.StatusNotFound, "block not found", err.Error())
	case errors.Is(err, store.ErrConflict):
		WriteProblem(w, r, http.StatusConflict, "block name already exists", err.Error())
	default:
		h.logAndWriteInternalError(w, r, op, err)
	}
}

// logAndWriteInternalError logs err server-side -- the only record of it,
// since the client-facing response never repeats it -- and writes a
// generic 500 problem+json response with no error detail. This matches the
// Recovery middleware's convention (internal/api/middleware/recovery.go) of
// never leaking internal error strings (e.g. raw sqlite/sqlx driver
// errors) in a response body: an attacker or a curious client learning
// schema/driver internals from a 500 body is a real information leak, and
// silently returning them without logging would mean an operator has no
// record of what actually failed. op identifies which handler code path
// produced the error (e.g. "list_blocks", "update_block_lookup").
func (h *Handlers) logAndWriteInternalError(w http.ResponseWriter, r *http.Request, op string, err error) {
	h.logInternalError(r, op, err)
	WriteProblem(w, r, http.StatusInternalServerError, "internal server error", "")
}

// logInternalError is logAndWriteInternalError's log-only half, for a
// caller that must NOT turn err into a 500 -- see e.g. UpdateBlock's and
// DeleteBlock's own DeleteFutureOccurrenceSnapshots calls: their primary
// mutation already succeeded and is authoritative by the time this runs,
// so a failure here (a best-effort consistency step, and one a retry --
// the call is naturally idempotent -- repeats for free) must not turn a
// successful write into a client-facing failure. Same log shape as
// logAndWriteInternalError, just without the response.
func (h *Handlers) logInternalError(r *http.Request, op string, err error) {
	h.d.Logger.Error("internal error",
		"op", op,
		"request_id", RequestIDFromContext(r.Context()),
		"error", err,
	)
}

// writeJSON writes body as an application/json response with the given
// status code.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// validateRequiredSpecFields checks the gen.BlockSpec fields the OpenAPI
// schema marks required (name, cron, duration, channel_id) that are also
// plain (non-pointer) Go strings, so a client that omits them decodes to a
// zero value rather than tripping a JSON decode error.
//
// duration doesn't need a check here: the CUE scheduler schema constrains
// it to `int & >0`, so a zero duration is a genuine CUE validation failure
// (confirmed by probing blockio.ValidateBlocks directly). name/cron/
// channel_id are different: their CUE fields are bare `string` types with
// no non-empty constraint, so an empty value round-trips through
// blockio.RenderYAML/ValidateBlocks and passes CUE validation cleanly --
// also confirmed by probing. Left unchecked, a POST/PUT body that omits a
// required string field would silently create/update a block with an
// empty name/cron/channel_id instead of being rejected, which violates the
// OpenAPI contract's `required: [name, cron, duration, channel_id]` for
// BlockSpec. This check closes that gap; it does not attempt to duplicate
// or replace CUE validation for anything CUE already covers correctly.
func validateRequiredSpecFields(spec gen.BlockSpec) error {
	var missing []string
	if spec.Name == "" {
		missing = append(missing, "name")
	}
	if spec.Cron == "" {
		missing = append(missing, "cron")
	}
	if spec.ChannelId == "" {
		missing = append(missing, "channel_id")
	}
	if len(missing) > 0 {
		return fmt.Errorf("failed to validate blocks: missing required field(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// validateSeriesShowTitles closes a deferred CUE-validation gap (tracked
// since Task 9, closed here): cmd/schema/config.cue types show_title as a
// bare `string` with no non-empty constraint, so a series block whose
// SeriesConfig.ShowTitle is "" round-trips through blockio.RenderYAML and
// passes blockio.ValidateBlocks/ParseYAML cleanly -- CUE has no way to
// express "non-empty" on that field today. An empty show_title is useless
// downstream (scheduler.Engine looks up series state and episodes by show
// title, see internal/scheduler/engine.go), so both places that hand a
// scheduler.Block to the store reject it explicitly:
//   - blocks CRUD (CreateBlock/UpdateBlock in this file), on the block
//     fromGen produces from the request body, alongside
//     validateRequiredSpecFields.
//   - YAML import (ImportBlocks, importexport.go), on each block
//     blockio.ParseYAML returns.
//
// Non-series blocks (b.Type != scheduler.BlockTypeSeries) are unaffected;
// their Series slice is expected to be empty and is not inspected.
func validateSeriesShowTitles(b scheduler.Block) error {
	if b.Type != scheduler.BlockTypeSeries {
		return nil
	}
	for _, sc := range b.Series {
		if sc.ShowTitle == "" {
			return fmt.Errorf("failed to validate blocks: series block %q has a series entry with an empty show_title", b.Name)
		}
	}
	return nil
}

// fromGen converts a gen.BlockSpec (the API wire shape) into a
// scheduler.Block (the domain type persisted by the store and validated by
// blockio.ValidateBlocks). It is reused by Task 14 (import/export).
//
// # CUE-default normalization
//
// blockio.ValidateBlocks works by re-marshaling a []scheduler.Block to YAML
// and validating that YAML against the CUE scheduler schema
// (cmd/schema/scheduler.cue). CUE's "field absent -> apply *default"
// semantics only fire when a field is missing from the YAML document; a Go
// zero value that gets marshaled explicitly (e.g. an empty string for an
// enum-typed field, or 0 for a field constrained to `int & >0`) is an
// explicit, invalid value as far as CUE is concerned, and fails the
// disjunction/bound check instead of defaulting (see the Task 5 report,
// "Surprises" #1, and confirmed again here by probing blockio.ValidateBlocks
// directly). gen.BlockSpec/gen.SeriesConfig/gen.SeriesFallback represent
// "not provided" as a nil pointer (all are optional/omitempty fields), so
// fromGen uses that nil-ness -- not zero-value-ness -- to decide when to
// apply the same default CUE would have applied to an absent field. A field
// the caller explicitly sent as an invalid value (e.g. `"type": "bogus"`)
// is passed through unchanged and left for ValidateBlocks to reject; only
// an *omitted* field is defaulted here.
//
// Fields normalized (mirrors cmd/schema/scheduler.cue's #Block defaults):
//   - Type: nil -> scheduler.BlockTypeFilter (CUE: "filter"|"series"|*"filter")
//   - SeriesConfig.OnComplete: nil -> scheduler.CompletionActionContinue
//     (CUE: "continue"|"restart"|"disable"|*"continue")
//   - SeriesConfig.StartSeason: nil -> 1 (CUE: int & >0 | *1)
//   - SeriesConfig.StartEpisode: nil -> 1 (CUE: int & >0 | *1)
//   - SeriesFallback.Mode: nil, when a fallback object is present at all ->
//     scheduler.FallbackModeRedistribute (CUE: "redistribute"|"filler"|*"redistribute")
//
// Every other optional field (priority, max_duration_overflow_minutes,
// filter.*, filler.*, series[].max_runs, series[].skip_episodes, ...) has a
// CUE type a Go zero value already satisfies without defaulting (e.g.
// `int | *10` accepts a bare 0 same as any other int, `bool | *false`
// accepts a bare false), so no normalization is needed for validation to
// pass. Note this means fromGen does NOT reproduce true CUE-default parity
// for those fields (e.g. an omitted priority ends up 0, not CUE's
// documented default of 10) -- only the minimum needed for ValidateBlocks
// to accept a reasonable request body. See the Task 5 report for why full
// parity is out of scope.
func fromGen(spec gen.BlockSpec) scheduler.Block {
	blockType := scheduler.BlockTypeFilter
	if spec.Type != nil {
		blockType = scheduler.BlockType(*spec.Type)
	}

	b := scheduler.Block{
		Type:                       blockType,
		Name:                       spec.Name,
		Cron:                       spec.Cron,
		Duration:                   spec.Duration,
		ChannelID:                  spec.ChannelId,
		Priority:                   intFromPtr(spec.Priority),
		MaxDurationOverflowMinutes: intFromPtr(spec.MaxDurationOverflowMinutes),
	}

	if spec.Filter != nil {
		b.Filter = filterFromGen(*spec.Filter)
	}
	if spec.Filler != nil {
		b.Filler = fillerFromGen(*spec.Filler)
	}
	if spec.Series != nil {
		series := make([]scheduler.SeriesConfig, 0, len(*spec.Series))
		for _, sc := range *spec.Series {
			series = append(series, seriesConfigFromGen(sc))
		}
		b.Series = series
	}
	if spec.Fallback != nil {
		b.Fallback = fallbackFromGen(*spec.Fallback)
	}

	return b
}

func intFromPtr(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func filterFromGen(f gen.Filter) scheduler.Filter {
	out := scheduler.Filter{}
	if f.TitlePattern != nil {
		out.TitlePattern = *f.TitlePattern
	}
	if f.Genres != nil {
		out.Genres = *f.Genres
	}
	if f.Ratings != nil {
		out.Ratings = *f.Ratings
	}
	out.YearFrom = intFromPtr(f.YearFrom)
	out.YearTo = intFromPtr(f.YearTo)
	out.MinDuration = intFromPtr(f.MinDuration)
	out.MaxDuration = intFromPtr(f.MaxDuration)
	if f.Tags != nil {
		out.Tags = *f.Tags
	}
	return out
}

func fillerFromGen(f gen.FillerConfig) scheduler.FillerConfig {
	out := scheduler.FillerConfig{}
	if f.Enabled != nil {
		out.Enabled = *f.Enabled
	}
	if f.FillerListId != nil {
		out.FillerListID = *f.FillerListId
	}
	out.MaxFillerTime = intFromPtr(f.MaxFillerTime)
	out.MinGapTime = intFromPtr(f.MinGapTime)
	return out
}

func seriesConfigFromGen(sc gen.SeriesConfig) scheduler.SeriesConfig {
	onComplete := scheduler.CompletionActionContinue
	if sc.OnComplete != nil {
		onComplete = scheduler.CompletionAction(*sc.OnComplete)
	}
	startSeason := 1
	if sc.StartSeason != nil {
		startSeason = *sc.StartSeason
	}
	startEpisode := 1
	if sc.StartEpisode != nil {
		startEpisode = *sc.StartEpisode
	}

	out := scheduler.SeriesConfig{
		ShowTitle:        sc.ShowTitle,
		EpisodesPerBlock: sc.EpisodesPerBlock,
		StartSeason:      startSeason,
		StartEpisode:     startEpisode,
		OnComplete:       onComplete,
		MaxRuns:          intFromPtr(sc.MaxRuns),
	}
	if sc.SkipEpisodes != nil {
		out.SkipEpisodes = *sc.SkipEpisodes
	}
	return out
}

func fallbackFromGen(f gen.SeriesFallback) scheduler.SeriesFallback {
	mode := scheduler.FallbackModeRedistribute
	if f.Mode != nil {
		mode = scheduler.FallbackMode(*f.Mode)
	}
	out := scheduler.SeriesFallback{Mode: mode}
	if f.FillerFilter != nil {
		out.FillerFilter = filterFromGen(*f.FillerFilter)
	}
	return out
}

// toGen converts a store.BlockRecord (the persisted domain representation)
// into a gen.BlockRecord (the API wire shape). It is reused by Task 14
// (import/export). Unlike fromGen, toGen has no CUE-default concerns -- it
// only ever reads already-valid, already-stored data -- so it simply
// presents every populated (non-zero) optional field as a pointer and
// leaves unpopulated ones nil, which keeps the JSON response free of
// spurious empty sub-objects (e.g. an unused "filter": {} on a series
// block).
func toGen(rec store.BlockRecord) gen.BlockRecord {
	return gen.BlockRecord{
		Id:        rec.ID,
		Name:      rec.Name,
		Enabled:   rec.Enabled,
		Spec:      specToGen(rec.Spec),
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}
}

func specToGen(b scheduler.Block) gen.BlockSpec {
	specType := gen.BlockSpecType(b.Type)
	priority := b.Priority
	overflow := b.MaxDurationOverflowMinutes

	spec := gen.BlockSpec{
		Name:                       b.Name,
		Cron:                       b.Cron,
		Duration:                   b.Duration,
		ChannelId:                  b.ChannelID,
		Type:                       &specType,
		Priority:                   &priority,
		MaxDurationOverflowMinutes: &overflow,
	}

	if !isZeroFilter(b.Filter) {
		f := filterToGen(b.Filter)
		spec.Filter = &f
	}
	if b.Filler != (scheduler.FillerConfig{}) {
		f := fillerToGen(b.Filler)
		spec.Filler = &f
	}
	if len(b.Series) > 0 {
		items := make([]gen.SeriesConfig, 0, len(b.Series))
		for _, sc := range b.Series {
			items = append(items, seriesConfigToGen(sc))
		}
		spec.Series = &items
	}
	if !isZeroFallback(b.Fallback) {
		fb := fallbackToGen(b.Fallback)
		spec.Fallback = &fb
	}

	return spec
}

func isZeroFilter(f scheduler.Filter) bool {
	return f.TitlePattern == "" && len(f.Genres) == 0 && len(f.Ratings) == 0 &&
		f.YearFrom == 0 && f.YearTo == 0 && f.MinDuration == 0 && f.MaxDuration == 0 &&
		len(f.Tags) == 0
}

func isZeroFallback(f scheduler.SeriesFallback) bool {
	return f.Mode == "" && isZeroFilter(f.FillerFilter)
}

func filterToGen(f scheduler.Filter) gen.Filter {
	out := gen.Filter{}
	if f.TitlePattern != "" {
		titlePattern := f.TitlePattern
		out.TitlePattern = &titlePattern
	}
	if len(f.Genres) > 0 {
		genres := f.Genres
		out.Genres = &genres
	}
	if len(f.Ratings) > 0 {
		ratings := f.Ratings
		out.Ratings = &ratings
	}
	if f.YearFrom != 0 {
		yearFrom := f.YearFrom
		out.YearFrom = &yearFrom
	}
	if f.YearTo != 0 {
		yearTo := f.YearTo
		out.YearTo = &yearTo
	}
	if f.MinDuration != 0 {
		minDuration := f.MinDuration
		out.MinDuration = &minDuration
	}
	if f.MaxDuration != 0 {
		maxDuration := f.MaxDuration
		out.MaxDuration = &maxDuration
	}
	if len(f.Tags) > 0 {
		tags := f.Tags
		out.Tags = &tags
	}
	return out
}

func fillerToGen(f scheduler.FillerConfig) gen.FillerConfig {
	enabled := f.Enabled
	out := gen.FillerConfig{Enabled: &enabled}
	if f.FillerListID != "" {
		fillerListID := f.FillerListID
		out.FillerListId = &fillerListID
	}
	if f.MaxFillerTime != 0 {
		maxFillerTime := f.MaxFillerTime
		out.MaxFillerTime = &maxFillerTime
	}
	if f.MinGapTime != 0 {
		minGapTime := f.MinGapTime
		out.MinGapTime = &minGapTime
	}
	return out
}

func seriesConfigToGen(sc scheduler.SeriesConfig) gen.SeriesConfig {
	onComplete := gen.SeriesConfigOnComplete(sc.OnComplete)
	startSeason := sc.StartSeason
	startEpisode := sc.StartEpisode

	out := gen.SeriesConfig{
		ShowTitle:        sc.ShowTitle,
		EpisodesPerBlock: sc.EpisodesPerBlock,
		StartSeason:      &startSeason,
		StartEpisode:     &startEpisode,
		OnComplete:       &onComplete,
	}
	if len(sc.SkipEpisodes) > 0 {
		skipEpisodes := sc.SkipEpisodes
		out.SkipEpisodes = &skipEpisodes
	}
	if sc.MaxRuns != 0 {
		maxRuns := sc.MaxRuns
		out.MaxRuns = &maxRuns
	}
	return out
}

func fallbackToGen(f scheduler.SeriesFallback) gen.SeriesFallback {
	mode := gen.SeriesFallbackMode(f.Mode)
	out := gen.SeriesFallback{Mode: &mode}
	if !isZeroFilter(f.FillerFilter) {
		ff := filterToGen(f.FillerFilter)
		out.FillerFilter = &ff
	}
	return out
}
