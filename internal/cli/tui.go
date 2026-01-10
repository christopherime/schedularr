package cli

import (
	"fmt"
	"os"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	tea "github.com/charmbracelet/bubbletea"
	"gopkg.in/yaml.v3"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive terminal interface for managing schedule",
	Run: func(cmd *cobra.Command, args []string) {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			fmt.Printf("Error parsing config: %v\n", err)
			os.Exit(1)
		}

		// Initial check to ensure we have a config file to write back to
		configFile := viper.ConfigFileUsed()
		if configFile == "" {
			fmt.Println("No config file found. Please create one first (e.g. config.yaml.example -> .schedularr.yaml)")
			os.Exit(1)
		}

		m := tui.NewModel(&cfg)
		p := tea.NewProgram(m, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
			os.Exit(1)
		}

		// Save config back
		// Note: This overrides comments in the YAML file
		data, err := yaml.Marshal(&cfg)
		if err != nil {
			fmt.Printf("Error marshaling config: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(configFile, data, 0644); err != nil {
			fmt.Printf("Error saving config to %s: %v\n", configFile, err)
			os.Exit(1)
		}
		
		fmt.Printf("Configuration saved to %s\n", configFile)
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
