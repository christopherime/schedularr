package cmd

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/external/tunarr"
	"github.com/geekxflood/schedularr/internal/store"
	"github.com/geekxflood/schedularr/internal/tui"
	"github.com/spf13/cobra"
)

var (
	cfgFile   string
	appConfig *config.Config
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "schedularr",
	Short: "Automated content scheduling for Tunarr TV channels",
	Long: `Schedularr automates content scheduling for Tunarr TV channels using
cron-based recurring blocks with advanced filtering.

Running without a subcommand launches the interactive TUI.

Features:
  • Cron-based scheduling with flexible time blocks
  • Advanced content filtering (genre, rating, year, duration)
  • Priority-based block scheduling
  • CUE schema validation for configurations
  • Interactive TUI for block editing
  • Series-based sequential episode progression`,

	Example: `# Launch interactive TUI (default)
  schedularr

  # Generate a new scheduler configuration
  schedularr scheduler init my-schedule.yaml

  # Validate configuration files
  schedularr validate config.yaml

  # Generate and apply schedules
  schedularr generate --scheduler my-schedule.yaml --apply`,

	// Run TUI by default when no subcommand is provided
	Run: func(_ *cobra.Command, _ []string) {
		if err := runTUI(); err != nil {
			fmt.Printf("%s %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags - update help text to reflect new defaults
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		fmt.Sprintf("config file (default is $HOME/%s/%s, env: %s)",
			config.DefaultConfigDir, config.DefaultConfigFile, config.EnvConfigPath))

	rootCmd.AddCommand(healthCmd) // Add health command to root
}

// initConfig reads in config file using CUE-based loading.
// Supports ${VAR_NAME} syntax for environment variable interpolation in config files.
// Priority: --config flag > SCHEDULARR_CONFIG env > legacy locations > $HOME/.schedularr/config.yaml
func initConfig() {
	// Use new path resolution with priority: flag > env > legacy > default
	configPath := config.FindConfigFile(cfgFile)

	if configPath != "" {
		// Load config using CUE validation and defaults
		cfg, err := config.Load(configPath)
		if err != nil {
			// Config file exists but failed to load - this is an error
			return
		}
		appConfig = cfg
	}

	// Ensure directories exist for schedules and logs
	paths, err := config.ResolvePaths(cfgFile)
	if err == nil {
		_ = config.EnsureDirectories(paths)
	}
}

// getConfig returns the loaded application config.
// If no config was loaded, it returns nil.
func getConfig() *config.Config {
	return appConfig
}


// runTUI launches the interactive terminal interface.
func runTUI() error {
	cfg := getConfig()
	if cfg == nil {
		return fmt.Errorf("config not loaded - ensure config.yaml exists")
	}

	// Load scheduler config
	schedCfg, err := config.FindSchedulerConfig(cfg, schedulerFile)
	if err != nil {
		return fmt.Errorf("failed to load scheduler config: %w", err)
	}

	// Determine the scheduler file path for saving
	schedulerPath := schedulerFile
	if schedulerPath == "" {
		schedulerPath = cfg.GetString("scheduler_file")
	}
	if schedulerPath == "" {
		if _, err := os.Stat("scheduler.yaml"); err == nil {
			schedulerPath = "scheduler.yaml"
		}
	}

	// Try to open the store (optional - if it fails, TUI works without series progress feature)
	var st *store.Store
	dbPath := config.DatabasePath(cfg)
	if dbPath != "" {
		st, _ = store.New(dbPath)
		if st != nil {
			defer st.Close()
		}
	}

	// Create Tunarr client (optional - if config is incomplete, TUI falls back to block-only mode)
	var tunarrClient *tunarr.Client
	tunarrCfg := config.TunarrConfig(cfg)
	if tunarrCfg.URL != "" {
		tunarrClient = tunarr.NewClient(tunarrCfg)
	}

	// Load timezone from config
	var tz *time.Location
	timezone := config.LogTimezone(cfg)
	if timezone != "" {
		tz, _ = time.LoadLocation(timezone)
	}
	if tz == nil {
		tz = time.Local
	}

	// Create logger for TUI (debug level for visibility)
	logger := newLogger(config.LogLevel(cfg), config.LogFormat(cfg))

	// Convert scheduler config to blocks
	blocks := config.SchedulerBlocks(schedCfg)

	opts := tui.ModelOptions{
		TunarrClient: tunarrClient,
		AppConfig:    cfg,
		Logger:       logger,
		Timezone:     tz,
		SchedulesDir: config.SchedulesDir(cfg),
	}

	m := tui.NewModel(blocks, st, schedulerPath, opts)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}
