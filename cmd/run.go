package cmd

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
)

var (
	runInterval time.Duration
	runOnce     bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the scheduling daemon",
	Long:  `Starts Schedularr in daemon mode, continuously generating and applying schedules based on configuration.`,
	Run: func(_ *cobra.Command, _ []string) {
		runDaemon()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().DurationVarP(&runInterval, "interval", "i", 6*time.Hour, "Interval between schedule updates")
	runCmd.Flags().BoolVar(&runOnce, "once", false, "Run once and exit")
	runCmd.Flags().StringVar(&schedulerFile, "scheduler", "", "Path to scheduler config file (default: scheduler.yaml)")
}

func runDaemon() {
	cfg := getConfig()
	if cfg == nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s config not loaded\n", errorStyle.Render("✗ Error:"))
		os.Exit(1)
	}

	appLogger := newLogger(config.LogLevel(cfg), config.LogFormat(cfg))

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🚀 Starting Schedularr daemon (Interval: %v)", runInterval)))

	metrics.RegisterMetrics()
	startMetricsServer(cfg, appLogger)

	runDaemonLoop(cfg)
}

func startMetricsServer(cfg *config.Config, appLogger *slog.Logger) {
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/healthz", healthCheckHandler)

		listenAddr := fmt.Sprintf(":%d", config.MetricsPort(cfg))
		appLogger.Info("Prometheus metrics and health check exposed", "address", listenAddr)

		server := &http.Server{
			Addr:              listenAddr,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}

		if err := server.ListenAndServe(); err != nil {
			appLogger.Error("Failed to start metrics server", "error", err)
			os.Exit(1)
		}
	}()
}

func healthCheckHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func runDaemonLoop(cfg *config.Config) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	ticker := time.NewTicker(runInterval)
	defer ticker.Stop()

	runJob(cfg)

	if runOnce {
		return
	}

	appLogger := newLogger(config.LogLevel(cfg), config.LogFormat(cfg))
	for {
		select {
		case <-ticker.C:
			runJob(cfg)
		case sig := <-sigChan:
			handleSignal(sig, appLogger)
			if sig != syscall.SIGHUP {
				return
			}
		}
	}
}

func handleSignal(sig os.Signal, appLogger *slog.Logger) {
	if sig == syscall.SIGHUP {
		appLogger.Info("Received SIGHUP signal")
		return
	}
	appLogger.Info("Received signal, shutting down...", "signal", sig)
}

func runJob(cfg *config.Config) {
	appLogger := newLogger(config.LogLevel(cfg), config.LogFormat(cfg))
	appLogger.Info("Running schedule update...")
	// Always apply in daemon mode
	if err := ProcessSchedule(cfg, schedulerFile, true, false); err != nil {
		appLogger.Error("Job failed", "error", err)
	} else {
		appLogger.Info("Job completed successfully")
	}
}
