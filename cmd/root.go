package cmd

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cfgFile string

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
  • Interactive TUI for block editing
  • Series-based sequential episode progression`,

	Example: `# Generate a new scheduler configuration
  schedularr scheduler init my-schedule.yaml

  # Validate configuration files
  schedularr validate config.yaml
  schedularr validate scheduler.yaml

  # Generate schedules
  schedularr generate --scheduler my-schedule.yaml --apply

  # Interactive TUI
  schedularr tui`,
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

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.schedularr.yaml)")

	rootCmd.AddCommand(healthCmd) // Add health command to root
}

// initConfig reads in config file and ENV variables if set.
// Supports ${VAR_NAME} syntax for environment variable interpolation in config files.
func initConfig() {
	viper.AutomaticEnv() // read in environment variables that match

	configPath := cfgFile
	if configPath == "" {
		// Try to find config in default locations
		configPath = findConfigFile()
	}

	if configPath != "" {
		// Read config file with environment variable interpolation
		if err := readConfigWithEnvInterpolation(configPath); err != nil {
			// Silently ignore missing config files
			return
		}
	}
}

// findConfigFile searches for config file in default locations.
func findConfigFile() string {
	// Check current directory first
	if _, err := os.Stat("config.yaml"); err == nil {
		return "config.yaml"
	}
	if _, err := os.Stat(".schedularr.yaml"); err == nil {
		return ".schedularr.yaml"
	}

	// Check home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	homePath := filepath.Join(home, ".schedularr.yaml")
	if _, err := os.Stat(homePath); err == nil {
		return homePath
	}

	return ""
}

// readConfigWithEnvInterpolation reads a config file and expands ${VAR_NAME} references.
func readConfigWithEnvInterpolation(configPath string) error {
	// Read the raw config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	// Expand environment variables using ${VAR_NAME} syntax
	expanded := os.ExpandEnv(string(data))

	// Set config type based on extension
	ext := filepath.Ext(configPath)
	switch ext {
	case ".json":
		viper.SetConfigType("json")
	case ".yaml", ".yml":
		viper.SetConfigType("yaml")
	default:
		viper.SetConfigType("yaml")
	}

	// Read expanded config into Viper
	return viper.ReadConfig(bytes.NewBufferString(expanded))
}
