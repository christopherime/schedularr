package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal interface for managing schedule",
	Long: `Interactive terminal interface for managing scheduling blocks.

Use ctrl+s to save changes to the scheduler configuration file.
Changes are only persisted when explicitly saved.`,
	Run: func(_ *cobra.Command, _ []string) {
		if err := runTUI(); err != nil {
			fmt.Printf("%s %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
