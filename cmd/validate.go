package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/christopherime/schedularr/internal/blockio"
	"github.com/christopherime/schedularr/internal/config"
	"github.com/christopherime/schedularr/internal/cueconfig"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/spf13/cobra"
)

var warningStyle = style{code: "1;33"} // Bold yellow

var validateCmd = &cobra.Command{
	Use:   "validate [file]",
	Short: "Validate configuration files against CUE schema",
	Long: `Validate configuration files against the embedded CUE schema.

Supports validating:
- Application configuration files (config.yaml, .schedularr.yaml)
- Scheduler configuration files (scheduler.yaml)
- Combined configuration files

The validator will automatically detect the file type and apply
the appropriate schema validation.

Examples:
  schedularr validate config.yaml
  schedularr validate scheduler.yaml
  schedularr validate ~/.schedularr.yaml`,
	Args: cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		filePath := args[0]

		if err := validateFile(filePath); err != nil {
			fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Validation failed:"), err)
			os.Exit(1)
		}

		fmt.Printf("%s %s\n", successStyle.Render("✓ Validation passed:"), filePath)
	},
}

func validateFile(filePath string) error {
	// Clean the file path to prevent directory traversal issues
	cleanPath := filepath.Clean(filePath)

	// Read the file
	// #nosec G304 - file path is provided by user via CLI argument
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Determine which schema to use based on filename
	baseName := filepath.Base(cleanPath)
	if strings.Contains(baseName, "scheduler") {
		return validateSchedulerFile(data)
	}

	return validateAppConfigFile(data, cleanPath)
}

// validateSchedulerFile validates data as a block import file: strict
// decode, duplicate-name check, and CUE schema validation (see
// blockio.ParseYAML), plus the additional Tunarr channel-ID check. This is
// the same validation Bootstrap and any future import API run, so
// `schedularr validate` proves a file is safe to import before it ever
// reaches the store.
func validateSchedulerFile(data []byte) error {
	blocks, err := blockio.ParseYAML(data)
	if err != nil {
		return fmt.Errorf("scheduler validation error: %w", err)
	}

	if err := validateSchedulerSemantics(blocks); err != nil {
		return fmt.Errorf("scheduler semantic validation error: %w", err)
	}

	return nil
}

// validateAppConfigFile validates data as an application config file
// against the CUE config schema. Format (YAML/JSON) is determined by
// cleanPath's extension.
func validateAppConfigFile(data []byte, cleanPath string) error {
	ext := strings.ToLower(filepath.Ext(cleanPath))
	format := "yaml"
	if ext == ".json" {
		format = "json"
	}

	validator := cueconfig.NewValidator()
	if err := validator.ValidateConfig(data, format); err != nil {
		return fmt.Errorf("config validation error: %w", err)
	}

	return nil
}

func validateSchedulerSemantics(blocks []scheduler.Block) error {
	// Get app config if available
	appCfg := getConfig()
	if appCfg == nil {
		// No app config loaded, skip Tunarr validation
		return nil
	}

	tunarrCfg := config.TunarrConfig(appCfg)
	if tunarrCfg.URL == "" {
		// No Tunarr configured, skip validation
		return nil
	}

	// Create client
	client := tunarr.NewClient(tunarrCfg)

	// Fetch channels
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	channels, err := client.GetChannels(ctx)
	if err != nil {
		// If we can't connect, maybe we warn?
		fmt.Fprintf(os.Stderr, "%s Could not connect to Tunarr for channel validation: %v\n", warningStyle.Render("! Warning:"), err)
		return nil
	}

	// Create a map for O(1) lookup
	channelMap := make(map[string]bool)
	for _, ch := range channels {
		channelMap[ch.ID] = true
	}

	// Check blocks
	var invalidChannels []string
	for _, block := range blocks {
		if block.ChannelID != "" && !channelMap[block.ChannelID] {
			invalidChannels = append(invalidChannels, fmt.Sprintf("Block '%s' references unknown channel '%s'", block.Name, block.ChannelID))
		}
	}

	if len(invalidChannels) > 0 {
		return fmt.Errorf("invalid channels detected:\n  - %s", strings.Join(invalidChannels, "\n  - "))
	}

	return nil
}

func init() {
	rootCmd.AddCommand(validateCmd)
}
