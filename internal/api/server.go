package api

import (
	"log/slog"
	"net/http"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/store"
)

// Deps holds the dependencies Handlers needs to serve requests. It starts
// minimal; Tasks 12 and 13 add Tunarr and schedule-runner interfaces here.
type Deps struct {
	Store   *store.Store
	Logger  *slog.Logger
	Version string
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

// GetHistory implements gen.ServerInterface.
func (h *Handlers) GetHistory(w http.ResponseWriter, r *http.Request, _ gen.GetHistoryParams) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "getHistory pending")
}

// ListSeriesState and PatchSeriesState implement gen.ServerInterface. See
// state.go.

// ListChannels implements gen.ServerInterface.
func (h *Handlers) ListChannels(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "listChannels pending")
}

// GetStatus implements gen.ServerInterface.
func (h *Handlers) GetStatus(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, http.StatusNotImplemented, "not implemented", "getStatus pending")
}
