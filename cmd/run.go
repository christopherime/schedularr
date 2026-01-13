package cmd

import (
	"fmt"
	"log/slog"
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
	cfg := loadDaemonConfig()
	appLogger := logging.NewLogger(cfg.Log.Level, cfg.Log.Format)

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🚀 Starting Schedularr daemon (Interval: %v)", runInterval)))

	metrics.RegisterMetrics()
	startMetricsServer(&cfg, appLogger)
	setupConfigWatcher(&cfg, appLogger)

	runDaemonLoop(&cfg)
}

func loadDaemonConfig() config.Config {
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error parsing config:"), err)
		os.Exit(1)
	}
	return cfg
}

func startMetricsServer(cfg *config.Config, appLogger *slog.Logger) {
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/healthz", healthCheckHandler)

		listenAddr := fmt.Sprintf(":%d", cfg.MetricsPort)
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

func setupConfigWatcher(cfg *config.Config, appLogger *slog.Logger) {
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		handleConfigChange(cfg, appLogger, e)
	})
}

func handleConfigChange(cfg *config.Config, appLogger *slog.Logger, e fsnotify.Event) {
	appLogger.Info("Config file changed, reloading...", "file", e.Name)

	var newCfg config.Config
	if err := viper.Unmarshal(&newCfg); err != nil {
		appLogger.Error("Failed to re-unmarshal config", "error", err)
		return
	}

	if err := validateNewConfig(&newCfg, appLogger); err != nil {
		return
	}

	*cfg = newCfg
	appLogger.Info("Configuration reloaded and validated successfully")
}

func validateNewConfig(newCfg *config.Config, appLogger *slog.Logger) error {
	validator := cueconfig.NewValidator()

	if err := validator.ValidateConfigStruct(newCfg); err != nil {
		appLogger.Error("New application config is invalid, keeping old config", "error", err)
		return err
	}

	if newCfg.SchedulerFile != "" {
		return validateSchedulerFile(newCfg, validator, appLogger)
	}

	return validateInlineScheduler(newCfg, validator, appLogger)
}

func validateSchedulerFile(newCfg *config.Config, validator *cueconfig.SchemaValidator, appLogger *slog.Logger) error {
	schedCfg, err := config.LoadSchedulerConfig(newCfg, newCfg.SchedulerFile)
	if err != nil {
		appLogger.Error("New scheduler config file is invalid, keeping old config", "error", err)
		return err
	}

	if err := validator.ValidateSchedulerStruct(schedCfg); err != nil {
		appLogger.Error("New scheduler config is invalid, keeping old config", "error", err)
		return err
	}

	newCfg.Scheduler = *schedCfg
	return nil
}

func validateInlineScheduler(newCfg *config.Config, validator *cueconfig.SchemaValidator, appLogger *slog.Logger) error {
	if err := validator.ValidateSchedulerStruct(&newCfg.Scheduler); err != nil {
		appLogger.Error("New inline scheduler config is invalid, keeping old config", "error", err)
		return err
	}
	return nil
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

	appLogger := logging.NewLogger(cfg.Log.Level, cfg.Log.Format)
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
		appLogger.Info("Received SIGHUP, triggering config reload (Viper.OnConfigChange should handle it)")
		return
	}
	appLogger.Info("Received signal, shutting down...", "signal", sig)
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
