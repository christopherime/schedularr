package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	apply         bool
	schedulerFile string
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate schedule based on rules",
	Run: func(_ *cobra.Command, _ []string) {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			fmt.Printf("Error parsing config: %v\n", err)
			os.Exit(1)
		}

		// Load scheduler configuration
		schedCfg, err := config.LoadSchedulerConfig(&cfg, schedulerFile)
		if err != nil {
			fmt.Printf("Error loading scheduler config: %v\n", err)
			os.Exit(1)
		}

		if len(schedCfg.Blocks) == 0 {
			fmt.Println("No scheduling blocks configured")
			os.Exit(1)
		}

		client := tunarr.NewClient(cfg.Tunarr)

		// Fetch source content
		programs, err := client.GetPrograms()
		if err != nil {
			fmt.Printf("Error fetching content: %v\n", err)
		}

		engine := scheduler.NewEngine(client, schedCfg.Blocks)

		start := time.Now()
		end := start.Add(24 * time.Hour) // Plan for next 24h

		plan, err := engine.GenerateForTimeRange(start, end, programs)
		if err != nil {
			fmt.Printf("Error generating schedule: %v\n", err)
			os.Exit(1)
		}

		for cid, items := range plan {
			fmt.Printf("Channel %s: %d items scheduled\n", cid, len(items))
			for _, item := range items {
				fmt.Printf(" - %s (%d min)\n", item.Title, item.Duration/60000)
			}

			if apply {
				fmt.Printf("Applying schedule to channel %s...\n", cid)
				if err := client.UpdateSchedule(cid, items); err != nil {
					fmt.Printf("Failed to update schedule for channel %s: %v\n", cid, err)
				} else {
					fmt.Printf("Successfully updated schedule for channel %s\n", cid)
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().BoolVar(&apply, "apply", false, "Apply generated schedule to Tunarr channels")
	generateCmd.Flags().StringVar(&schedulerFile, "scheduler", "", "Path to scheduler config file (default: scheduler.yaml)")
}
