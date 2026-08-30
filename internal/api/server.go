package api

import (
	"log/slog"
	"time"

	"github.com/christopherime/schedularr/internal/api/gen"
	"github.com/christopherime/schedularr/internal/service"
	"github.com/christopherime/schedularr/internal/store"
)

// Deps holds the dependencies Handlers needs to serve requests.
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

	// Sched is the schedule-generation boundary used by GenerateSchedule,
	// ApplySchedule, and GetSchedule (see schedule.go). Production wiring
	// passes a *service.Runner, which satisfies ScheduleRunner as-is;
	// tests pass a fake.
	Sched service.ScheduleRunner

	// NextCronTick reports when the serve command's cron loop will next
	// generate and apply a schedule -- GetStatus surfaces it as
	// Status.next_cron_tick. nil (the whole func, or its result) means no
	// cron loop is running / no tick is scheduled, and the field is
	// omitted; tests and any future loop-less mode simply leave this
	// unset. cmd/serve.go wires it to the loop's own tick tracker.
	NextCronTick func() *time.Time

	// Media is the media-discovery boundary used by ListMediaShows and
	// GetMediaMeta (see media.go). Production wiring passes the same
	// *service.Runner as Sched -- MediaAPI is a separate, narrower
	// interface (matching TunarrAPI/ScheduleRunner's precedent of one
	// interface per concern) rather than folding MediaShows/MediaMeta
	// into ScheduleRunner, but one concrete Runner satisfies both, which
	// is what guarantees media discovery reuses Run's exact fetch+cache
	// path instead of standing up a second one. Tests pass a fake.
	Media MediaAPI
}

// Handlers implements gen.ServerInterface.
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

// ImportBlocks and ExportBlocks implement gen.ServerInterface. See
// importexport.go.

// GenerateSchedule, ApplySchedule, and GetSchedule implement
// gen.ServerInterface. See schedule.go.

// GetHistory implements gen.ServerInterface. See history.go.

// ListSeriesState and PatchSeriesState implement gen.ServerInterface. See
// state.go.

// ListChannels and GetStatus implement gen.ServerInterface. See tunarr.go.

// ListMediaShows and GetMediaMeta implement gen.ServerInterface. See
// media.go.
