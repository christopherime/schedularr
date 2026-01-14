package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/store"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
  • Last aired timestamp`,

	Example: "schedularr state export backup-2026-01-12.json",
	Args:    cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		outputFile := args[0]

		// Load config
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
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
	RunE: func(_ *cobra.Command, args []string) error {
		inputFile := filepath.Clean(args[0])

		// Read file
		// #nosec G304 - user-provided file path is intentional for import command
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		// Unmarshal JSON
		var states []scheduler.SeriesState
		if err := json.Unmarshal(data, &states); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}

		// Load config
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
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
	RunE: func(_ *cobra.Command, args []string) error {
		showTitle := args[0]

		// Load config
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
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

		// Reset state
		ctx := context.Background()
		if err := s.ResetSeriesState(ctx, showTitle); err != nil {
			return fmt.Errorf("failed to reset state: %w", err)
		}

		fmt.Printf("Reset series state for \"%s\" to S01E01\n", showTitle)
		return nil
	},
}

var stateBackupCmd = &cobra.Command{
	Use:   "backup <file>",
	Short: "Backup the entire database to a file",
	Long: `Create a safe, transactional backup of the entire SQLite database.
This includes both series progression states and schedule history.

The backup is performed using SQLite's VACUUM INTO command, ensuring
consistency even if the database is in use.

Example:
  schedularr state backup full-backup-2026-01-12.db`,
	Args: cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		outputFile := args[0]

		// Load config
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
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

		// Perform backup
		ctx := context.Background()
		if err := s.Backup(ctx, outputFile); err != nil {
			return fmt.Errorf("failed to backup database: %w", err)
		}

		fmt.Printf("Database backed up successfully to %s\n", outputFile)
		return nil
	},
}

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all series states",
	Long: `List all series progression states showing current episode and completion status.

Example:
  schedularr state list`,
	RunE: func(_ *cobra.Command, _ []string) error {
		// Load config
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			return fmt.Errorf("failed to parse config: %w", err)
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

		// Export states
		ctx := context.Background()
		states, err := s.ExportAllSeriesStates(ctx)
		if err != nil {
			return fmt.Errorf("failed to list states: %w", err)
		}

		if len(states) == 0 {
			fmt.Println("No series states found.")
			return nil
		}

		// Print header
		fmt.Printf("%-40s %-15s %-10s %-20s\n", "Show Title", "Current", "Status", "Last Aired")
		fmt.Println(string(make([]byte, 90)))

		// Print each state
		for _, state := range states {
			current := fmt.Sprintf("S%02dE%02d", state.CurrentSeason, state.CurrentEpisode)
			status := "In Progress"
			if state.Completed {
				status = "Completed"
			}
			lastAired := "Never"
			if state.LastAired != nil && !state.LastAired.IsZero() {
				lastAired = state.LastAired.Format(time.RFC3339)
			}

			fmt.Printf("%-40s %-15s %-10s %-20s\n", state.ShowTitle, current, status, lastAired)
		}

		fmt.Printf("\nTotal: %d series\n", len(states))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stateCmd)
	stateCmd.AddCommand(stateExportCmd)
	stateCmd.AddCommand(stateImportCmd)
	stateCmd.AddCommand(stateResetCmd)
	stateCmd.AddCommand(stateBackupCmd)
	stateCmd.AddCommand(stateListCmd)
}
