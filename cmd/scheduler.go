package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/christopherime/schedularr/internal/cueconfig"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	// Flags for scheduler init command
	blockName      string
	blockCron      string
	blockDuration  int
	blockChannelID string
	blockPriority  int
)

var schedulerCmd = &cobra.Command{
	Use:   "scheduler",
	Short: "Author a scheduler.yaml import file",
	Long: `Commands for creating a scheduler.yaml file for the store's first-run
import (see internal/blockio.Bootstrap). Blocks live in the SQLite store
once imported; validate the generated file with "schedularr validate" and
manage blocks afterward via the "/api/v1/blocks" HTTP API or a future block
CLI, not by re-editing scheduler.yaml.`,
}

var schedulerInitCmd = &cobra.Command{
	Use:   "init [filename]",
	Short: "Create a new scheduler configuration file",
	Long: `Generate a scheduler configuration file from the CUE schema with defaults.
If no filename is provided, creates 'scheduler.yaml' in the current directory.

The generated file will contain example blocks with all default values
extracted from the CUE schema.

You can override default block values using flags, e.g., --name "My TV Show"`,
	Example: `schedularr scheduler init
  schedularr scheduler init my-schedule.yaml --name "Morning Cartoons" --channel-id "kids-channel"
  schedularr scheduler init schedule.json --cron "0 8 * * *" --duration 180`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		filename := "scheduler.yaml"
		if len(args) > 0 {
			filename = args[0]
		}

		// Check if file already exists
		if _, err := os.Stat(filename); err == nil {
			fmt.Printf("Error: File %s already exists. Remove it first or choose a different name.\n", filename)
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
		data, err := validator.GenerateScheduler(format)
		if err != nil {
			fmt.Printf("Error generating scheduler config: %v\n", err)
			os.Exit(1)
		}

		// Unmarshal generated data to apply flag overrides
		var schedCfg scheduler.Config
		if err := yaml.Unmarshal(data, &schedCfg); err != nil {
			fmt.Printf("Error unmarshaling generated scheduler config: %v\n", err)
			os.Exit(1)
		}

		// Apply flag overrides to the first block if it exists
		if len(schedCfg.Blocks) > 0 {
			if blockName != "" {
				schedCfg.Blocks[0].Name = blockName
			}
			if blockCron != "" {
				schedCfg.Blocks[0].Cron = blockCron
			}
			if blockDuration != 0 {
				schedCfg.Blocks[0].Duration = blockDuration
			}
			if blockChannelID != "" {
				schedCfg.Blocks[0].ChannelID = blockChannelID
			}
			if blockPriority != 0 {
				schedCfg.Blocks[0].Priority = blockPriority
			}
		}

		// Marshal back to YAML/JSON
		if format == "yaml" {
			data, err = yaml.Marshal(schedCfg)
		} else {
			data, err = json.MarshalIndent(schedCfg, "", "  ")
		}
		if err != nil {
			fmt.Printf("Error re-marshaling scheduler config with overrides: %v\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(filename, data, 0o600); err != nil {
			fmt.Printf("Error creating file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Created scheduler configuration file: %s\n", filename)
		fmt.Printf("Format: %s\n", format)
		fmt.Printf("\nEdit this file to configure your scheduling blocks, then place it at your\n")
		fmt.Printf("scheduler_file path (default: scheduler.yaml) and run:\n")
		fmt.Printf("  schedularr generate\n")
		fmt.Printf("It will be imported into the block store on first run.\n")
	},
}

func init() {
	rootCmd.AddCommand(schedulerCmd)
	schedulerCmd.AddCommand(schedulerInitCmd)

	// Flags for scheduler init command
	schedulerInitCmd.Flags().StringVar(&blockName, "name", "", "Name for the initial scheduling block")
	schedulerInitCmd.Flags().StringVar(&blockCron, "cron", "", "Cron expression for the initial scheduling block")
	schedulerInitCmd.Flags().IntVar(&blockDuration, "duration", 0, "Duration in minutes for the initial scheduling block")
	schedulerInitCmd.Flags().StringVar(&blockChannelID, "channel-id", "", "Channel ID for the initial scheduling block")
	schedulerInitCmd.Flags().IntVar(&blockPriority, "priority", 0, "Priority for the initial scheduling block")
}
