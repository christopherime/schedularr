package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/store"
	"github.com/spf13/cobra"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "Manage series state (export, import, reset)",
	Long: `Manage series progression state for series-based scheduling blocks.

The state command allows you to:
  • Export all series states to JSON for backup
  • Import series states from JSON to restore or migrate
  • Reset a series to start from the beginning

Examples:
  # Export all series states to a file
  schedularr state export states.json

  # Import series states from a file
  schedularr state import states.json

  # Reset a specific series to S01E01
  schedularr state reset "Show Title"

  # List all series states
  schedularr state list`,
}

var stateExportCmd = &cobra.Command{
	Use:   "export <file>",
	Short: "Export all series states to JSON",
	Long: `Export all series progression states to a JSON file for backup or migration.

The exported file contains all series states including:
  • Show title
  • Current season and episode
  • Completion status
  • Last aired timestamp

Example:
  schedularr state export backup-2026-01-12.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFile := args[0]

		// Get database path from config or use default
		dbPath := filepath.Join(os.Getenv("HOME"), ".schedularr.db")
		if cfg.Database != "" {
			dbPath = cfg.Database
		}

		// Open store
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer s.Close()

		// Export states
		ctx := context.Background()
		states, err := s.ExportAllSeriesStates(ctx)
		if err != nil {
			return fmt.Errorf("failed to export states: %w", err)
		}

		// Marshal to JSON
		data, err := json.MarshalIndent(states, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal states: %w", err)
		}

		// Write to file
		if err := os.WriteFile(outputFile, data, 0600); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		fmt.Printf("Exported %d series states to %s\n", len(states), outputFile)
		return nil
	},
}

var stateImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import series states from JSON",
	Long: `Import series progression states from a JSON file.

This will update existing series states or create new ones.
Existing states with the same show title will be overwritten.

Example:
  schedularr state import backup-2026-01-12.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		inputFile := args[0]

		// Read file
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Unmarshal JSON
		var states []scheduler.SeriesState
		if err := json.Unmarshal(data, &states); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}

		// Get database path from config or use default
		dbPath := filepath.Join(os.Getenv("HOME"), ".schedularr.db")
		if cfg.Database != "" {
			dbPath = cfg.Database
		}

		// Open store
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer s.Close()

		// Import states
		ctx := context.Background()
		if err := s.ImportSeriesStates(ctx, states); err != nil {
			return fmt.Errorf("failed to import states: %w", err)
		}

		fmt.Printf("Imported %d series states from %s\n", len(states), inputFile)
		return nil
	},
}

var stateResetCmd = &cobra.Command{
	Use:   "reset <show-title>",
	Short: "Reset a series to start from the beginning",
	Long: `Reset a series progression state to S01E01 (not completed).

This is useful when you want to restart a series from the beginning.

Example:
  schedularr state reset "My Favorite Show"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		showTitle := args[0]

		// Get database path from config or use default
		dbPath := filepath.Join(os.Getenv("HOME"), ".schedularr.db")
		if cfg.Database != "" {
			dbPath = cfg.Database
		}

		// Open store
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer s.Close()

		// Reset state
		ctx := context.Background()
		if err := s.ResetSeriesState(ctx, showTitle); err != nil {
			return fmt.Errorf("failed to reset state: %w", err)
		}

		fmt.Printf("Reset series state for \"%s\" to S01E01\n", showTitle)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stateCmd)
	stateCmd.AddCommand(stateExportCmd)
	stateCmd.AddCommand(stateImportCmd)
	stateCmd.AddCommand(stateResetCmd)
}

