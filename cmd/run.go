package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	runInterval time.Duration
	runOnce     bool
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the scheduling daemon",
	Long:  `Starts Schedularr in daemon mode, continuously generating and applying schedules based on configuration.`, 
	Run: func(_ *cobra.Command, _ []string) {
		runDaemon()
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().DurationVarP(&runInterval, "interval", "i", 6*time.Hour, "Interval between schedule updates")
	runCmd.Flags().BoolVar(&runOnce, "once", false, "Run once and exit")
	runCmd.Flags().StringVar(&schedulerFile, "scheduler", "", "Path to scheduler config file (default: scheduler.yaml)")
}

func runDaemon() {
	// Parse config
	var cfg config.Config
	if err := viper.Unmarshal(&cfg); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error parsing config:"), err)
		os.Exit(1)
	}

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🚀 Starting Schedularr daemon (Interval: %v)", runInterval)))

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Run loop
	ticker := time.NewTicker(runInterval)
	defer ticker.Stop()

	// Run immediately first
	runJob(&cfg)

	if runOnce {
		return
	}

	for {
		select {
		case <-ticker.C:
			runJob(&cfg)
		case sig := <-sigChan:
			fmt.Printf("\n%s %v, shutting down...\n", infoStyle.Render("Received signal"), sig)
			return
		}
	}
}

func runJob(cfg *config.Config) {
	fmt.Printf("\n[%s] %s\n", time.Now().Format(time.RFC3339), infoStyle.Render("Running schedule update..."))
	// Always apply in daemon mode
	if err := ProcessSchedule(cfg, schedulerFile, true, false); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "[%s] %s %v\n", time.Now().Format(time.RFC3339), errorStyle.Render("✗ Job failed:"), err)
	} else {
		fmt.Printf("[%s] %s\n", time.Now().Format(time.RFC3339), successStyle.Render("✓ Job completed successfully"))
	}
}
