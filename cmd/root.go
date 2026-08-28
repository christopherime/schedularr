package cmd

import (
	"fmt"
	"os"

	"github.com/christopherime/schedularr/internal/config"
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

Features:
  • Cron-based scheduling with flexible time blocks
  • Advanced content filtering (genre, rating, year, duration)
  • Priority-based block scheduling
  • CUE schema validation for configurations
  • Series-based sequential episode progression`,

	Example: `# Generate a new scheduler configuration (imported into the store on first run)
  schedularr scheduler init scheduler.yaml

  # Validate configuration files
  schedularr validate config.yaml

  # Generate and apply schedules
  schedularr generate --apply --yes`,
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
