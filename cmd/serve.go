package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/christopherime/schedularr/internal/api"
	"github.com/christopherime/schedularr/internal/blockio"
	"github.com/christopherime/schedularr/internal/config"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/metrics"
	"github.com/christopherime/schedularr/internal/service"
	"github.com/christopherime/schedularr/internal/store"
)

// Version is the build version reported by GET /api/v1/status
// (gen.Status.Version, see internal/api/tunarr.go's GetStatus). It defaults
// to "dev" for a source build; a release build overrides it via
// `-ldflags "-X github.com/christopherime/schedularr/cmd.Version=..."`.
var Version = "dev"

// cronInterval is how often the cron loop regenerates and applies the
// schedule -- the same default the removed `schedularr run` command's
// --interval flag had. Unlike run, serve has no per-invocation --interval
// flag: it's meant to be started once by a process supervisor (systemd,
// Docker, ...) and left running, not re-invoked per tick, so there's no
// CLI-ergonomics reason to expose this as a flag. See task-15-report.md
// for the tradeoff if this needs to become configurable later.
const cronInterval = 6 * time.Hour

// shutdownTimeout bounds how long graceful shutdown waits for in-flight
// HTTP requests to finish before giving up.
const shutdownTimeout = 15 * time.Second

var (
	serveListen         string
	serveInsecureNoAuth bool
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run the HTTP API server and the cron scheduling loop",
	Long: `Starts Schedularr's HTTP API server (blocks CRUD, schedule
generate/apply, history, series state, channels, status, and the OpenAPI
contract at /openapi.json) together with the cron scheduling loop, in a
single long-lived process.

This replaces the old "run" daemon command: the API and the scheduler now
share one process, one store connection, and one graceful-shutdown path.

Endpoints:
  GET  /healthz        Process liveness (no dependency checks)
  GET  /readyz          Liveness plus a store round-trip (SELECT 1)
  GET  /metrics          Prometheus metrics
  GET  /openapi.json     The OpenAPI 3.0 contract as JSON
  /api/v1/*               The API itself (bearer-token authenticated)

Authentication:
  /api/v1/* requires "Authorization: Bearer <token>". The token comes from
  the SCHEDULARR_API_TOKEN environment variable (preferred) or the
  api.token config key -- the environment variable always wins when both
  are set. --insecure-no-auth disables the check entirely; use only for
  local development.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runServe(cmd)
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
	serveCmd.Flags().StringVar(&serveListen, "listen", ":8484", "Address for the HTTP API server to listen on")
	serveCmd.Flags().BoolVar(&serveInsecureNoAuth, "insecure-no-auth", false,
		"Skip bearer-token auth on /api/v1/* (local development only, never for a real deployment)")
}

// runServe is serveCmd's implementation: load config, assemble the store,
// Tunarr client, service.Runner, and API router, then run the HTTP server
// and the cron loop until a shutdown signal arrives.
func runServe(cmd *cobra.Command) error {
	cfg := getConfig()
	if cfg == nil {
		return errors.New("config not loaded")
	}

	logger := newLogger(config.LogLevel(cfg), config.LogFormat(cfg))

	listen := serveListen
	if !cmd.Flags().Changed("listen") {
		if v := config.APIListen(cfg); v != "" {
			listen = v
		}
	}
	insecureNoAuth := serveInsecureNoAuth || config.APIInsecureNoAuth(cfg)

	st, err := store.New(config.DatabasePath(cfg))
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() { _ = st.Close() }()

	bootstrapCtx := context.Background()
	imported, err := blockio.Bootstrap(bootstrapCtx, st, config.SchedulerFilePath(cfg), logger)
	if err != nil {
		return fmt.Errorf("failed to bootstrap blocks: %w", err)
	}
	if imported > 0 {
		logger.Info("bootstrap imported scheduling blocks", "count", imported)
	}

	timezone := config.LogTimezone(cfg)
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return fmt.Errorf("invalid timezone %q in app config: %w", timezone, err)
	}

	client := tunarr.NewClient(config.TunarrConfig(cfg))
	runner := service.NewRunner(st, client, logger, loc)

	metrics.RegisterMetrics()

	router, err := api.NewRouter(
		api.Config{Token: config.APIToken(cfg), InsecureNoAuth: insecureNoAuth},
		api.Deps{Store: st, Tunarr: client, Sched: runner, Logger: logger, Version: Version},
	)
	if err != nil {
		return fmt.Errorf("failed to build api router: %w", err)
	}

	ln, err := net.Listen("tcp", listen)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listen, err)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("api server listening", "address", ln.Addr().String())
	return serveUntil(sigCtx, ln, router, runner, logger)
}

// serveUntil runs the HTTP server (bound to ln) and the cron scheduling
// loop side by side until ctx is canceled, then shuts both down gracefully:
// http.Server.Shutdown (bounded by shutdownTimeout) followed by the cron
// loop's own context cancellation. It's factored out of runServe so tests
// can drive it directly against a real net.Listener (e.g. one bound to
// ":0") without going through cobra, config loading, or a live Tunarr/store
// setup.
func serveUntil(ctx context.Context, ln net.Listener, handler http.Handler, runner service.ScheduleRunner, logger *slog.Logger) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErrCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
			return
		}
		serveErrCh <- nil
	}()

	// cronCtx is derived from ctx via context.WithoutCancel: it carries
	// whatever values ctx has but is NOT canceled when ctx is. That's the
	// point -- the cron loop keeps running through the HTTP shutdown below
	// and is only stopped explicitly, via cancelCron, once
	// http.Server.Shutdown returns.
	cronCtx, cancelCron := context.WithCancel(context.WithoutCancel(ctx))
	cronDone := make(chan struct{})
	go func() {
		defer close(cronDone)
		runCronLoop(cronCtx, runner, logger)
	}()

	var result error
	select {
	case <-ctx.Done():
		logger.Info("serve: shutdown requested")
	case err := <-serveErrCh:
		result = err
		if result != nil {
			logger.Error("serve: http server failed", "error", result)
		}
	}

	// shutdownCtx must not inherit ctx's cancellation -- ctx is already
	// Done by the time we reach here on the normal shutdown path, since
	// that's what triggered it. context.WithoutCancel plus a fresh timeout
	// is the standard pattern for "keep going gracefully after the thing
	// that told you to stop already fired."
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("serve: http server shutdown error", "error", err)
		if result == nil {
			result = fmt.Errorf("http server shutdown: %w", err)
		}
	}

	cancelCron()
	<-cronDone

	return result
}

// runCronLoop runs one schedule generate-and-apply cycle immediately, then
// again every cronInterval, until ctx is canceled. This is the same shape
// the removed cmd/run.go's runDaemonLoop had (immediate run, then
// ticker-driven, cancelable) -- minus SIGHUP (serve has no config-reload
// story) and minus --once (serve is always long-running, never a one-shot
// invocation).
func runCronLoop(ctx context.Context, runner service.ScheduleRunner, logger *slog.Logger) {
	runScheduleTick(ctx, runner, logger)

	ticker := time.NewTicker(cronInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			runScheduleTick(ctx, runner, logger)
		case <-ctx.Done():
			return
		}
	}
}

// runScheduleTick runs a single generate-and-apply cycle for the next
// day's schedule, mirroring cmd/generate.go's ProcessSchedule call
// (service.Options{Days: 1, Apply: true}) -- the same call the removed
// daemon loop made via ProcessSchedule(cfg, true, false) on every tick. A
// failure is logged and the loop continues; it doesn't stop the server or
// retry early, matching the old daemon's behavior of just waiting for the
// next tick.
func runScheduleTick(ctx context.Context, runner service.ScheduleRunner, logger *slog.Logger) {
	logger.Info("cron: running schedule generation")
	if _, err := runner.Run(ctx, service.Options{Days: 1, Apply: true}); err != nil {
		logger.Error("cron: schedule generation failed", "error", err)
		return
	}
	logger.Info("cron: schedule generation completed")
}
