package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/store"
	"github.com/geekxflood/schedularr/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal interface for managing schedule",
	Long: `Interactive terminal interface for managing scheduling blocks.

Use ctrl+s to save changes to the scheduler configuration file.
Changes are only persisted when explicitly saved.`,
	Run: func(_ *cobra.Command, _ []string) {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			fmt.Printf("Error parsing config: %v\n", err)
			os.Exit(1)
		}

		// Determine the scheduler file path for saving
		schedulerFile := cfg.SchedulerFile
		if schedulerFile == "" {
			// Check if scheduler.yaml exists, otherwise use default
			if _, err := os.Stat("scheduler.yaml"); err == nil {
				schedulerFile = "scheduler.yaml"
			}
		}

		// Try to open the store (optional - if it fails, TUI works without series progress feature)
		var st *store.Store
		if cfg.Database != "" {
			st, _ = store.New(cfg.Database)
			if st != nil {
				defer st.Close()
			}
		}

		m := tui.NewModel(&cfg, st, schedulerFile)
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
