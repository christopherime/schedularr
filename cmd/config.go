package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/christopherime/schedularr/internal/cueconfig"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	tunarrURL          string
	tunarrAPIKey       string
	logLevel           string
	logFormat          string
	jellyfinURL        string
	jellyfinAPIKey     string
	jellyfinSyncLiveTV bool
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

You can override default values using flags, e.g., --tunarr-url.

Examples:
  schedularr config generate
  schedularr config generate my-config.yaml --tunarr-url "http://my-tunarr:8000"
  schedularr config generate config.json --log-level debug --jellyfin-sync-livetv`,
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

		// Unmarshal generated data to apply flag overrides
		var cfg map[string]any
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			fmt.Printf("%s Failed to unmarshal generated config: %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}

		// Apply flag overrides using map access
		applyConfigOverrides(cfg)

		// Marshal back to YAML/JSON
		if format == "yaml" {
			data, err = yaml.Marshal(cfg)
		} else {
			data, err = json.MarshalIndent(cfg, "", "  ")
		}
		if err != nil {
			fmt.Printf("%s Failed to re-marshal config with overrides: %v\n", errorStyle.Render("✗ Error:"), err)
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
		cfg := getConfig()
		if cfg == nil {
			fmt.Printf("%s No configuration loaded\n", errorStyle.Render("✗ Error:"))
			os.Exit(1)
		}

		output, err := cfg.ToYAML()
		if err != nil {
			fmt.Printf("%s Failed to marshal config to YAML: %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}

		fmt.Println(string(output))
	},
}

// applyConfigOverrides applies command-line flag overrides to the generated config map
func applyConfigOverrides(cfg map[string]any) {
	// Helper to ensure nested map exists
	ensureMap := func(parent map[string]any, key string) map[string]any {
		if m, ok := parent[key].(map[string]any); ok {
			return m
		}
		m := make(map[string]any)
		parent[key] = m
		return m
	}

	// Apply Tunarr overrides
	if tunarrURL != "" || tunarrAPIKey != "" {
		tunarr := ensureMap(cfg, "tunarr")
		if tunarrURL != "" {
			tunarr["url"] = tunarrURL
		}
		if tunarrAPIKey != "" {
			tunarr["api_key"] = tunarrAPIKey
		}
	}

	// Apply log overrides
	if logLevel != "" || logFormat != "" {
		log := ensureMap(cfg, "log")
		if logLevel != "" {
			log["level"] = logLevel
		}
		if logFormat != "" {
			log["format"] = logFormat
		}
	}

	// Apply Jellyfin overrides
	if jellyfinURL != "" || jellyfinAPIKey != "" || jellyfinSyncLiveTV {
		jellyfin := ensureMap(cfg, "jellyfin")
		if jellyfinURL != "" {
			jellyfin["url"] = jellyfinURL
		}
		if jellyfinAPIKey != "" {
			jellyfin["api_key"] = jellyfinAPIKey
		}
		if jellyfinSyncLiveTV {
			jellyfin["sync_live_tv"] = jellyfinSyncLiveTV
		}
	}
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGenerateCmd)
	configCmd.AddCommand(configDumpCmd) // Added config dump command

	// Add flags to configGenerateCmd
	configGenerateCmd.Flags().StringVar(&tunarrURL, "tunarr-url", "", "Override default Tunarr API URL")
	configGenerateCmd.Flags().StringVar(&tunarrAPIKey, "tunarr-api-key", "", "Override default Tunarr API Key")
	configGenerateCmd.Flags().StringVar(&logLevel, "log-level", "", "Override default log level (debug, info, warn, error)")
	configGenerateCmd.Flags().StringVar(&logFormat, "log-format", "", "Override default log format (text, json)")
	configGenerateCmd.Flags().StringVar(&jellyfinURL, "jellyfin-url", "", "Override default Jellyfin API URL")
	configGenerateCmd.Flags().StringVar(&jellyfinAPIKey, "jellyfin-api-key", "", "Override default Jellyfin API Key")
	configGenerateCmd.Flags().BoolVar(&jellyfinSyncLiveTV, "jellyfin-sync-livetv", false, "Override default Jellyfin Live TV sync (set to true)")
}
