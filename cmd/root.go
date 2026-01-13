package cmd

import (
	"os"

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

// initConfig reads in config file and ENV variables if set
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory and current directory
		viper.AddConfigPath(home)
		viper.AddConfigPath(".")
		viper.SetConfigType("yaml")
		viper.SetConfigName(".schedularr")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in (silently)
	_ = viper.ReadInConfig()
}
