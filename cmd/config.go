package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekxflood/schedularr/internal/config" // Added import
	"github.com/geekxflood/schedularr/internal/cueconfig"
	"github.com/spf13/cobra"
	"github.com/spf13/viper" // Added import
	"gopkg.in/yaml.v3"       // Added import
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage application configuration",
	Long:  `Commands for generating and managing application configuration files.`,
}

var configGenerateCmd = &cobra.Command{
	Use:   "generate [filename]",
	Short: "Generate application configuration file from CUE schema",
	Long: `Generate an application configuration file from the embedded CUE schema with defaults.
If no filename is provided, creates 'config.yaml' in the current directory.

The generated file will contain all configuration options with their default values
extracted from the CUE schema.

Examples:
  schedularr config generate
  schedularr config generate my-config.yaml
  schedularr config generate config.json`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		filename := "config.yaml"
		if len(args) > 0 {
			filename = args[0]
		}

		// Check if file already exists
		if _, err := os.Stat(filename); err == nil {
			fmt.Printf("%s File %s already exists. Remove it first or choose a different name.\n",
				errorStyle.Render("✗ Error:"), filename)
			os.Exit(1)
		}

		// Determine format from extension
		ext := strings.ToLower(filepath.Ext(filename))
		format := "yaml"
		if ext == ".json" {
			format = "json"
		}

		// Generate from CUE schema
		validator := cueconfig.NewValidator()
		data, err := validator.GenerateConfig(format)
		if err != nil {
			fmt.Printf("%s %v\n", errorStyle.Render("✗ Error generating config:"), err)
			os.Exit(1)
		}

		if err := os.WriteFile(filename, data, 0o600); err != nil {
			fmt.Printf("%s %v\n", errorStyle.Render("✗ Error creating file:"), err)
			os.Exit(1)
		}

		fmt.Printf("%s %s\n", successStyle.Render("✓ Created configuration file:"), filename)
		fmt.Printf("Format: %s\n", format)
		fmt.Printf("\nEdit this file to configure Schedularr, then use:\n")
		fmt.Printf("  schedularr --config %s <command>\n", filename)
	},
}

var configDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dump the current effective configuration",
	Long: `Prints the currently active configuration (after loading from file,
environment variables, and applying defaults) to standard output in YAML format.
Useful for debugging.`,
	Args: cobra.NoArgs,
	Run: func(_ *cobra.Command, _ []string) {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			fmt.Printf("%s Failed to unmarshal config: %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}

		output, err := yaml.Marshal(cfg)
		if err != nil {
			fmt.Printf("%s Failed to marshal config to YAML: %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}

		fmt.Println(string(output))
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGenerateCmd)
	configCmd.AddCommand(configDumpCmd) // Added config dump command
}
