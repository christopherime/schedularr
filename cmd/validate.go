package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/geekxflood/schedularr/internal/cueconfig"
	"github.com/spf13/cobra"
)

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
	// Read the file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Determine format from extension
	ext := strings.ToLower(filepath.Ext(filePath))
	format := "yaml"
	if ext == ".json" {
		format = "json"
	}

	// Create validator
	validator := cueconfig.NewValidator()

	// Determine which schema to use based on filename
	baseName := filepath.Base(filePath)
	isScheduler := strings.Contains(baseName, "scheduler")

	if isScheduler {
		// Validate as scheduler config
		if err := validator.ValidateScheduler(data, format); err != nil {
			return fmt.Errorf("scheduler validation error: %w", err)
		}
	} else {
		// Validate as application config
		if err := validator.ValidateConfig(data, format); err != nil {
			return fmt.Errorf("config validation error: %w", err)
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

