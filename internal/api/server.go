package api

import (
	"log/slog"
	"net/http"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/store"
)

// Deps holds the dependencies Handlers needs to serve requests. It starts
// minimal; Task 13 adds a schedule-runner interface here.
type Deps struct {
	Store   *store.Store
	Logger  *slog.Logger
	Version string

	// Tunarr is the Tunarr API boundary used by ListChannels and GetStatus
	// (see tunarr.go). Production wiring passes a *tunarr.Client, which
	// satisfies TunarrAPI as-is; tests pass a fake. A nil Tunarr means
	// "Tunarr integration not configured" -- both handlers treat that as a
	// normal, expected state rather than a programming error, since a
	// Schedularr deployment need not run Tunarr at all.
	Tunarr TunarrAPI
}

// Handlers implements gen.ServerInterface. Every operation currently
// returns 501 problem+json; Tasks 8-15 replace the stubs one group at a time.
type Handlers struct {
	d Deps
}

// NewHandlers constructs Handlers backed by d.
func NewHandlers(d Deps) *Handlers {
	return &Handlers{d: d}
}

var _ gen.ServerInterface = (*Handlers)(nil)

// ListBlocks, CreateBlock, GetBlock, UpdateBlock, and DeleteBlock implement
// gen.ServerInterface. See blocks.go.

// ImportBlocks implements gen.ServerInterface.
func (h *Handlers) ImportBlocks(w http.ResponseWriter, r *http.Request, _ gen.ImportBlocksParams) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "importBlocks pending")
}

// ExportBlocks implements gen.ServerInterface.
func (h *Handlers) ExportBlocks(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "exportBlocks pending")
}

// GenerateSchedule implements gen.ServerInterface.
func (h *Handlers) GenerateSchedule(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "generateSchedule pending")
}

// ApplySchedule implements gen.ServerInterface.
func (h *Handlers) ApplySchedule(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "applySchedule pending")
}

// GetSchedule implements gen.ServerInterface.
func (h *Handlers) GetSchedule(w http.ResponseWriter, r *http.Request, _ gen.GetScheduleParams) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "getSchedule pending")
}

// GetHistory implements gen.ServerInterface. See history.go.

// ListSeriesState and PatchSeriesState implement gen.ServerInterface. See
// state.go.

// ListChannels and GetStatus implement gen.ServerInterface. See tunarr.go.
