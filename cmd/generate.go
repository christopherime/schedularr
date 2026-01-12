package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/logging"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/store"
	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	apply         bool
	schedulerFile string
	dryRun        bool
	verbose       bool
)

// Color styles
var (
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	errorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	infoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	warnStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate schedule based on rules",
	Long: `Generate programming schedules based on configured blocks.

This command:
1. Fetches available content from Tunarr libraries
2. Applies filtering rules from each block
3. Generates optimized schedules with conflict resolution
4. Optionally applies the schedule to Tunarr channels

Use --dry-run to preview schedules without applying them.
Use --verbose for detailed output including filtering and history.`,
	Run: func(_ *cobra.Command, _ []string) {
		var cfg config.Config
		if err := viper.Unmarshal(&cfg); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error: failed to parse config:"), err)
			os.Exit(1)
		}

		if err := ProcessSchedule(&cfg, schedulerFile, apply, dryRun); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}
	},
}

// ProcessSchedule generates and optionally applies the schedule.
func ProcessSchedule(cfg *config.Config, schedFile string, apply bool, dryRun bool) error {
	// Load scheduler configuration
	schedCfg, err := config.LoadSchedulerConfig(cfg, schedFile)
	if err != nil {
		return fmt.Errorf("failed to load scheduler config: %w", err)
	}

	if len(schedCfg.Blocks) == 0 {
		return errors.New("no scheduling blocks configured")
	}

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📋 Loaded %d scheduling block(s)", len(schedCfg.Blocks))))

	// Initialize Store
	st, err := store.New("schedularr.db")
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer st.Close()

	// Create Tunarr client
	client := tunarr.NewClient(cfg.Tunarr)

	// Fetch available content
	fmt.Println(infoStyle.Render("📡 Fetching content from Tunarr..."))
	programs := fetchAllContent(client)

	if len(programs) == 0 {
		fmt.Println(warnStyle.Render("⚠ No content available - using fallback GetPrograms()"))
		programs, err = client.GetPrograms(context.Background())
		if err != nil {
			return fmt.Errorf("failed to fetch programs: %w", err)
		}
	}

	fmt.Printf("%s\n", successStyle.Render(fmt.Sprintf("✓ Found %d program(s)", len(programs))))

	// Create logger
	logger := logging.NewLogger(cfg.Log.Level, cfg.Log.Format)

	loc, err := time.LoadLocation(cfg.Log.Timezone)
	if err != nil {
		return fmt.Errorf("invalid timezone '%s' in app config: %w", cfg.Log.Timezone, err)
	}

	// Create scheduling engine
	engine := scheduler.NewEngine(client, schedCfg.Blocks, st, logger, loc)

	// Generate schedule
	start := time.Now()
	end := start.Add(24 * time.Hour) // Plan for next 24h

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🗓️  Generating schedule from %s to %s in %s",
		start.Format("2006-01-02 15:04"),
		end.Format("2006-01-02 15:04"),
		cfg.Log.Timezone)))

	plan, err := engine.GenerateForTimeRange(start, end, programs)
	if err != nil {
		return fmt.Errorf("failed to generate schedule: %w", err)
	}

	// Display results
	displaySchedule(plan, verbose)

	// Flatten the schedule plan for applySchedule
	flattenedPlan := make(map[string][]tunarr.Program)
	for channelID, slots := range plan {
		for _, slot := range slots {
			flattenedPlan[channelID] = append(flattenedPlan[channelID], slot.Programs...)
		}
	}

	// Apply to Tunarr if requested
	if apply && !dryRun {
		if err := applySchedule(client, flattenedPlan); err != nil {
			return err
		}
		// Commit state changes to DB
		if err := engine.Commit(); err != nil {
			return fmt.Errorf("failed to commit state: %w", err)
		}
		return nil
	} else if dryRun {
		fmt.Println(infoStyle.Render("\n🔍 Dry run mode - schedule not applied"))
	} else {
		fmt.Println(infoStyle.Render("\n💡 Use --apply to push schedule to Tunarr"))
	}

	return nil
}

func fetchAllContent(client *tunarr.Client) []tunarr.Program {
	var allPrograms []tunarr.Program

	// Try to fetch from libraries
	libraries, err := client.GetLibraries(context.Background())
	if err != nil {
		if verbose {
			fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch libraries: %v", err)))
		}
		return allPrograms
	}

	if verbose {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📚 Found %d librar(y/ies)", len(libraries))))
	}

	for _, lib := range libraries {
		if verbose {
			fmt.Printf("  - %s (%s)\n", lib.Name, lib.Type)
		}

		programs, err := client.GetLibraryPrograms(context.Background(), lib.ID)
		if err != nil {
			if verbose {
				fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("    ⚠ Could not fetch programs from %s: %v", lib.Name, err)))
			}
			continue
		}

		if verbose {
			fmt.Printf("    ✓ %d programs\n", len(programs))
		}

		allPrograms = append(allPrograms, programs...)
	}

	return allPrograms
}

func displaySchedule(plan map[string][]scheduler.ScheduledSlot, _ bool) {
	if len(plan) == 0 {
		fmt.Println(warnStyle.Render("\n⚠ No schedule generated"))
		return
	}

	fmt.Println()
	fmt.Println(successStyle.Render("✓ Schedule Generated"))
	fmt.Println("=" + fmt.Sprintf("%80s", "=")[1:])

	totalProgramsScheduled := 0
	for channelID, slots := range plan {
		channelTotalDuration := int64(0)
		channelProgramCount := 0

		var output strings.Builder
		output.WriteString(fmt.Sprintf("\n📺 Channel %s:\n", channelID))

		w := tabwriter.NewWriter(&output, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "START TIME\tEND TIME\tBLOCK NAME\tPROGRAM\tDURATION\tTYPE\tSHOW\tSEASON\tEPISODE\n")
		fmt.Fprintf(w, "----------\t--------\t----------\t-------\t--------\t----\t----\t------\t-------\n")

		for _, slot := range slots {
			slotStartTime := slot.StartTime
			blockName := slot.Block.Name

			currentProgramTime := slotStartTime
			for _, p := range slot.Programs {
				programEndTime := currentProgramTime.Add(time.Duration(p.Duration) * time.Millisecond)
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
					currentProgramTime.Format("15:04"),
					programEndTime.Format("15:04"),
					blockName,
					p.Title,
					fmtDuration(p.Duration),
					p.Type,
					p.ShowTitle,
					p.Season,
					p.Episode,
				)
				currentProgramTime = programEndTime
				channelTotalDuration += p.Duration
				channelProgramCount++
			}
		}
		_ = w.Flush()
		fmt.Print(output.String())

		fmt.Printf("   Total items for channel: %d (%s)\n", channelProgramCount, fmtDuration(channelTotalDuration))
		totalProgramsScheduled += channelProgramCount
	}

	fmt.Printf("\n📊 Total: %d programs scheduled across %d channel(s)\n",
		totalProgramsScheduled, len(plan))
}

// fmtDuration converts duration in milliseconds to a human-readable string (e.g., 1h 30m).
func fmtDuration(ms int64) string {
	totalSeconds := ms / 1000
	hours := totalSeconds / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}


func applySchedule(client *tunarr.Client, plan map[string][]tunarr.Program) error {
	fmt.Println()
	fmt.Println(infoStyle.Render("🚀 Applying schedule to Tunarr..."))

	successCount := 0
	failCount := 0

	for cid, items := range plan {
		fmt.Printf("  📺 Channel %s...", cid)

		if err := client.UpdateSchedule(context.Background(), cid, items); err != nil {
			fmt.Print(errorStyle.Render(" ✗\n"))
			fmt.Printf("     %s %v\n", errorStyle.Render("Error:"), err)
			failCount++
		} else {
			fmt.Print(successStyle.Render(" ✓\n"))
			successCount++
		}
	}

	fmt.Println()
	if failCount == 0 {
		fmt.Printf("%s\n", successStyle.Render(fmt.Sprintf("✓ Successfully applied schedule to %d channel(s)", successCount)))
		return nil
	}

	fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Applied to %d channel(s), %d failed", successCount, failCount)))
	return fmt.Errorf("failed to apply schedule to %d channel(s)", failCount)
}

func init() {
	rootCmd.AddCommand(generateCmd)
	generateCmd.Flags().BoolVar(&apply, "apply", false, "Apply generated schedule to Tunarr channels")
	generateCmd.Flags().StringVar(&schedulerFile, "scheduler", "", "Path to scheduler config file (default: scheduler.yaml)")
}
