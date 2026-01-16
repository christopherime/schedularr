package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/store"
	"github.com/geekxflood/schedularr/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal interface for managing schedule",
	Long: `Interactive terminal interface for managing scheduling blocks.

Use ctrl+s to save changes to the scheduler configuration file.
Changes are only persisted when explicitly saved.`,
	Run: func(_ *cobra.Command, _ []string) {
		cfg := getConfig()
		if cfg == nil {
			fmt.Printf("%s config not loaded\n", errorStyle.Render("✗ Error:"))
			os.Exit(1)
		}

		// Load scheduler config
		schedCfg, err := config.FindSchedulerConfig(cfg, schedulerFile)
		if err != nil {
			fmt.Printf("%s Failed to load scheduler config: %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}

		// Determine the scheduler file path for saving
		schedulerPath := schedulerFile
		if schedulerPath == "" {
			schedulerPath = cfg.GetString("scheduler_file")
		}
		if schedulerPath == "" {
			// Check if scheduler.yaml exists, otherwise use default
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

		// Convert scheduler config to blocks
		blocks := config.SchedulerBlocks(schedCfg)

		m := tui.NewModel(blocks, st, schedulerPath)
		p := tea.NewProgram(m, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
			os.Exit(1)
		}

		// Note: Saving is now handled by ctrl+s in the TUI
		// Changes are only persisted when the user explicitly saves
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
