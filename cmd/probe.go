package cmd

import (
	"context"
	"log/slog"
	"time"

	"github.com/christopherime/schedularr/internal/events"
)

// defaultProbeInterval is how often serve asks Tunarr whether it is
// there. Frequent enough that the bezel's reading is worth trusting,
// infrequent enough to stay invisible in Tunarr's access log.
const defaultProbeInterval = 30 * time.Second

// probeTimeout bounds one reachability check, so a Tunarr that accepts
// connections but never answers cannot stall the prober past its own
// interval. Matches the API's own status-probe budget.
const probeTimeout = 5 * time.Second

// probeDeps is what the reachability prober needs. Check is injected
// rather than taking a Tunarr client directly, so a test can flip
// reachability without a live server.
type probeDeps struct {
	Hub      *events.Hub
	Interval time.Duration
	Logger   *slog.Logger
	Check    func(context.Context) bool
}

// startTunarrProbe watches Tunarr's reachability and publishes
// status.changed when it FLIPS -- never on every probe. A per-interval
// publish would wake every connected tab twice a minute to tell it
// nothing happened; publishing only on a transition is what makes the
// event's arrival meaningful.
//
// The first probe always publishes, so a tab that connects before any
// flip still learns the current reading without waiting for one.
//
// This exists because nothing else server-side notices Tunarr going away:
// the only Tunarr calls between applies are the ones a browser triggers,
// so without a prober status.changed could never fire.
func startTunarrProbe(ctx context.Context, p probeDeps) {
	if p.Hub == nil || p.Check == nil {
		return
	}
	if p.Interval <= 0 {
		p.Interval = defaultProbeInterval
	}
	if p.Logger == nil {
		p.Logger = slog.Default()
	}

	go func() {
		ticker := time.NewTicker(p.Interval)
		defer ticker.Stop()

		var last bool
		seeded := false

		for {
			now := p.Check(ctx)
			if !seeded || now != last {
				p.Hub.Publish(events.StatusChanged, map[string]any{"tunarr_reachable": now})
				if seeded {
					p.Logger.Info("tunarr reachability changed", "reachable", now)
				}
				last, seeded = now, true
			}

			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
