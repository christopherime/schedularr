package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/cueconfig"
	"github.com/geekxflood/schedularr/internal/logging"
	"github.com/geekxflood/schedularr/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	// Parse config
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error parsing config:"), err)
		os.Exit(1)
	}

	// Initialize structured logger
	appLogger := logging.NewLogger(cfg.Log.Level, cfg.Log.Format)

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🚀 Starting Schedularr daemon (Interval: %v)", runInterval)))

	// Register Prometheus metrics
	metrics.RegisterMetrics()

	// Start Prometheus metrics server in a goroutine
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		listenAddr := fmt.Sprintf(":%d", cfg.MetricsPort)
		appLogger.Info("Prometheus metrics and health check exposed", "address", listenAddr)
		server := &http.Server{
			Addr:              listenAddr,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		}
		err := server.ListenAndServe()
		if err != nil {
			appLogger.Error("Failed to start metrics server", "error", err)
			os.Exit(1) // Exit if metrics server fails to start
		}
	}()

	// Setup Viper to watch for config changes
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		appLogger.Info("Config file changed, reloading...", "file", e.Name)
		// Re-unmarshal the config
		var newCfg config.Config // Create a temporary config to unmarshal into for validation
		if err := viper.Unmarshal(&newCfg); err != nil {
			appLogger.Error("Failed to re-unmarshal config", "error", err)
			return
		}

		validator := cueconfig.NewValidator()
		// Validate main app config
		if err := validator.ValidateConfigStruct(&newCfg); err != nil {
			appLogger.Error("New application config is invalid, keeping old config", "error", err)
			return
		}
		// Validate scheduler config (if a scheduler file is specified)
		if newCfg.SchedulerFile != "" {
			// Load and validate the scheduler file
			schedCfg, err := config.LoadSchedulerConfig(&newCfg, newCfg.SchedulerFile)
			if err != nil {
				appLogger.Error("New scheduler config file is invalid, keeping old config", "error", err)
				return
			}
			if err := validator.ValidateSchedulerStruct(schedCfg); err != nil {
				appLogger.Error("New scheduler config is invalid, keeping old config", "error", err)
				return
			}
			newCfg.Scheduler = *schedCfg // Assign the validated scheduler config
		} else {
			// If inline scheduler config is used
			if err := validator.ValidateSchedulerStruct(&newCfg.Scheduler); err != nil {
				appLogger.Error("New inline scheduler config is invalid, keeping old config", "error", err)
				return
			}
		}

		// If all validations pass, update the main config
		cfg = newCfg
		appLogger.Info("Configuration reloaded and validated successfully")
	})

	// Setup signal handling for graceful shutdown and SIGHUP for reload
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	// Run loop
	ticker := time.NewTicker(runInterval)
	defer ticker.Stop()

	// Run immediately first
	runJob(&cfg)

	if runOnce {
		return
	}

	for {
		select {
		case <-ticker.C:
			runJob(&cfg)
		case sig := <-sigChan:
			if sig == syscall.SIGHUP {
				appLogger.Info("Received SIGHUP, triggering config reload (Viper.OnConfigChange should handle it)")
				// Viper's OnConfigChange is async, so we just log and let it handle the reload.
				// No need to call viper.ReadInConfig() here, as WatchConfig handles it.
				continue
			}
			appLogger.Info("Received signal, shutting down...", "signal", sig)
			return
		}
	}
}

func runJob(cfg *config.Config) {
	appLogger := logging.NewLogger(cfg.Log.Level, cfg.Log.Format)
	appLogger.Info("Running schedule update...")
	// Always apply in daemon mode
	if err := ProcessSchedule(cfg, schedulerFile, true, false); err != nil {
		appLogger.Error("Job failed", "error", err)
	} else {
		appLogger.Info("Job completed successfully")
	}
}
