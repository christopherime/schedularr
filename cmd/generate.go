package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/external/jellyfin"
	"github.com/geekxflood/schedularr/internal/external/tunarr"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/store"
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

	// Program type colors
	movieStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))            // Magenta
	episodeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))            // Blue
	trackStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))            // Cyan
	headerStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true) // White/Bold
	channelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true) // Green/Bold
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
	schedCfg, st, err := initializeScheduler(cfg, schedFile)
	if err != nil {
		return err
	}
	defer st.Close()

	client := tunarr.NewClient(cfg.Tunarr)
	programs, err := fetchAndValidateContent(cfg, client)
	if err != nil {
		return err
	}

	engine, err := createEngine(cfg, client, schedCfg.Blocks, st)
	if err != nil {
		return err
	}

	plan, err := generateSchedulePlan(cfg, engine, programs)
	if err != nil {
		return err
	}

	displaySchedule(plan, verbose)
	flattenedPlan := flattenSchedule(plan)

	return handleScheduleOutput(cfg, client, engine, flattenedPlan, scheduleOutputOptions{
		apply:  apply,
		dryRun: dryRun,
	})
}

func initializeScheduler(cfg *config.Config, schedFile string) (*scheduler.Config, *store.Store, error) {
	schedCfg, err := config.LoadSchedulerConfig(cfg, schedFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load scheduler config: %w", err)
	}

	if len(schedCfg.Blocks) == 0 {
		return nil, nil, errors.New("no scheduling blocks configured")
	}

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📋 Loaded %d scheduling block(s)", len(schedCfg.Blocks))))

	st, err := store.New("schedularr.db")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	return schedCfg, st, nil
}

func fetchAndValidateContent(cfg *config.Config, client *tunarr.Client) ([]tunarr.Program, error) {
	programs, err := fetchAllContent(cfg, client)
	if err != nil {
		return nil, err
	}

	fmt.Printf("%s\n", successStyle.Render(fmt.Sprintf("✓ Found %d program(s)", len(programs))))
	return programs, nil
}

func createEngine(cfg *config.Config, client *tunarr.Client, blocks []scheduler.Block, st *store.Store) (*scheduler.Engine, error) {
	logger := newLogger(cfg.Log.Level, cfg.Log.Format)

	loc, err := time.LoadLocation(cfg.Log.Timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone '%s' in app config: %w", cfg.Log.Timezone, err)
	}

	return scheduler.NewEngine(client, blocks, st, logger, loc), nil
}

func generateSchedulePlan(cfg *config.Config, engine *scheduler.Engine, programs []tunarr.Program) (map[string][]scheduler.ScheduledSlot, error) {
	start := time.Now()
	end := start.Add(24 * time.Hour)

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🗓️  Generating schedule from %s to %s in %s",
		start.Format("2006-01-02 15:04"),
		end.Format("2006-01-02 15:04"),
		cfg.Log.Timezone)))

	plan, err := engine.GenerateForTimeRange(start, end, programs)
	if err != nil {
		return nil, fmt.Errorf("failed to generate schedule: %w", err)
	}

	return plan, nil
}

func flattenSchedule(plan map[string][]scheduler.ScheduledSlot) map[string][]tunarr.Program {
	flattenedPlan := make(map[string][]tunarr.Program)
	for channelID, slots := range plan {
		for _, slot := range slots {
			flattenedPlan[channelID] = append(flattenedPlan[channelID], slot.Programs...)
		}
	}
	return flattenedPlan
}

type scheduleOutputOptions struct {
	apply  bool
	dryRun bool
}

func handleScheduleOutput(cfg *config.Config, client *tunarr.Client, engine *scheduler.Engine, flattenedPlan map[string][]tunarr.Program, opts scheduleOutputOptions) error {
	if opts.apply && !opts.dryRun {
		return applyScheduleAndSync(cfg, client, engine, flattenedPlan)
	}

	if opts.dryRun {
		displayDryRunSummary(flattenedPlan)
	} else {
		fmt.Println(infoStyle.Render("\n💡 Use --apply to push schedule to Tunarr"))
		fmt.Println(infoStyle.Render("   Use --dry-run to see what would be applied without making changes"))
	}

	return nil
}

func applyScheduleAndSync(cfg *config.Config, client *tunarr.Client, engine *scheduler.Engine, flattenedPlan map[string][]tunarr.Program) error {
	if err := applySchedule(client, flattenedPlan); err != nil {
		return err
	}

	if err := engine.Commit(); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}

	if cfg.Jellyfin.URL != "" && cfg.Jellyfin.SyncLiveTV {
		syncJellyfinLiveTV(cfg)
	}

	return nil
}

func syncJellyfinLiveTV(cfg *config.Config) {
	fmt.Println(infoStyle.Render("📺 Refreshing Jellyfin Live TV guide..."))
	jellyfinClient := jellyfin.NewClient(cfg.Jellyfin)
	if err := refreshJellyfinWithRetries(jellyfinClient); err != nil {
		fmt.Printf("%s %v\n", warnStyle.Render("⚠ Failed to refresh Jellyfin Live TV guide (non-fatal):"), err)
	} else {
		fmt.Printf("%s\n", successStyle.Render("✓ Jellyfin Live TV guide refreshed"))
	}
}

func refreshJellyfinWithRetries(client *jellyfin.Client) error {
	maxRetries := 3
	baseDelay := 2 * time.Second

	var err error
	for i := 0; i < maxRetries; i++ {
		// Use a short timeout for the request itself if possible, but client has 30s timeout.
		err = client.RefreshLiveTVGuide(context.Background())
		if err == nil {
			return nil
		}

		if i < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<i) // 2s, 4s, 8s
			fmt.Printf("   %s %v (retrying in %s)...\n", warnStyle.Render("⚠ Refresh failed:"), err, delay)
			time.Sleep(delay)
		}
	}
	return fmt.Errorf("after %d attempts: %w", maxRetries, err)
}

type channelStats struct {
	programCount  int
	totalDuration int64
	movies        int
	episodes      int
	tracks        int
}

func (cs *channelStats) incrementType(programType string) lipgloss.Style {
	switch programType {
	case "movie":
		cs.movies++
		return movieStyle
	case "episode":
		cs.episodes++
		return episodeStyle
	case "track":
		cs.tracks++
		return trackStyle
	default:
		return infoStyle
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

func formatProgramRow(w *tabwriter.Writer, currentTime time.Time, program tunarr.Program, blockName string, typeStyle lipgloss.Style) time.Time {
	programEndTime := currentTime.Add(time.Duration(program.Duration) * time.Millisecond)

	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%d\t%d\n",
		infoStyle.Render(currentTime.Format("15:04")),
		infoStyle.Render(programEndTime.Format("15:04")),
		blockName,
		truncateString(program.Title, 30),
		fmtDuration(program.Duration),
		typeStyle.Render(program.Type),
		truncateString(program.ShowTitle, 20),
		program.Season,
		program.Episode,
	)

	return programEndTime
}

func displayChannelSchedule(channelID string, slots []scheduler.ScheduledSlot) channelStats {
	stats := channelStats{}

	var output strings.Builder
	output.WriteString("\n" + channelStyle.Render("📺 Channel "+channelID) + "\n")

	w := tabwriter.NewWriter(&output, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprint(w, headerStyle.Render("START\tEND\tBLOCK\tPROGRAM\tDURATION\tTYPE\tSHOW\tS\tE\n"))
	_, _ = fmt.Fprint(w, headerStyle.Render(strings.Repeat("─", 90)+"\n"))

	for _, slot := range slots {
		currentTime := slot.StartTime
		for _, program := range slot.Programs {
			typeStyle := stats.incrementType(program.Type)
			currentTime = formatProgramRow(w, currentTime, program, slot.Block.Name, typeStyle)
			stats.totalDuration += program.Duration
			stats.programCount++
		}
	}

	_ = w.Flush()
	fmt.Print(output.String())
	displayChannelSummary(stats)

	return stats
}

func displayChannelSummary(stats channelStats) {
	summary := fmt.Sprintf("   Total: %d programs (%s)", stats.programCount, fmtDuration(stats.totalDuration))
	breakdown := formatTypeBreakdown(stats.movies, stats.episodes, stats.tracks)
	fmt.Println(infoStyle.Render(summary) + breakdown)
}

func formatTypeBreakdown(movies, episodes, tracks int) string {
	var parts []string
	if movies > 0 {
		parts = append(parts, movieStyle.Render(fmt.Sprintf(" • %d movies", movies)))
	}
	if episodes > 0 {
		parts = append(parts, episodeStyle.Render(fmt.Sprintf(" • %d episodes", episodes)))
	}
	if tracks > 0 {
		parts = append(parts, trackStyle.Render(fmt.Sprintf(" • %d tracks", tracks)))
	}
	return strings.Join(parts, "")
}

func displaySchedule(plan map[string][]scheduler.ScheduledSlot, _ bool) {
	if len(plan) == 0 {
		fmt.Println(warnStyle.Render("\n⚠ No schedule generated"))
		return
	}

	fmt.Println()
	fmt.Println(successStyle.Render("✓ Schedule Generated"))
	fmt.Println(headerStyle.Render(strings.Repeat("═", 100)))

	totalStats := channelStats{}
	for channelID, slots := range plan {
		channelStats := displayChannelSchedule(channelID, slots)
		totalStats.programCount += channelStats.programCount
		totalStats.movies += channelStats.movies
		totalStats.episodes += channelStats.episodes
		totalStats.tracks += channelStats.tracks
	}

	// Overall summary
	fmt.Println()
	fmt.Println(headerStyle.Render(strings.Repeat("═", 100)))
	fmt.Printf("📊 %s: %d programs across %d channel(s)\n",
		successStyle.Render("Total"),
		totalStats.programCount, len(plan))

	// Type breakdown
	typeBreakdown := formatTypeBreakdown(totalStats.movies, totalStats.episodes, totalStats.tracks)
	if typeBreakdown != "" {
		fmt.Println("   " + typeBreakdown)
	}
	fmt.Println()
}

// displayDryRunSummary shows what would be applied in dry-run mode
func displayDryRunSummary(plan map[string][]tunarr.Program) {
	fmt.Println()
	fmt.Println(warnStyle.Render("🔍 DRY RUN MODE"))
	fmt.Println(infoStyle.Render("The following changes would be applied to Tunarr:"))
	fmt.Println()

	totalPrograms := 0
	for channelID, programs := range plan {
		totalPrograms += len(programs)
		fmt.Printf("  %s %s\n", channelStyle.Render("📺 Channel:"), channelID)
		fmt.Printf("     Programs to schedule: %s\n", successStyle.Render(strconv.Itoa(len(programs))))

		// Calculate total duration
		totalDuration := int64(0)
		for _, p := range programs {
			totalDuration += p.Duration
		}
		fmt.Printf("     Total duration: %s\n", infoStyle.Render(fmtDuration(totalDuration)))

		// Show first few programs as preview
		previewCount := 3
		if len(programs) < previewCount {
			previewCount = len(programs)
		}
		if previewCount > 0 {
			fmt.Println("     Preview:")
			for i := 0; i < previewCount; i++ {
				p := programs[i]
				var typeStyle lipgloss.Style
				switch p.Type {
				case "movie":
					typeStyle = movieStyle
				case "episode":
					typeStyle = episodeStyle
				case "track":
					typeStyle = trackStyle
				default:
					typeStyle = infoStyle
				}
				fmt.Printf("       %d. %s (%s)\n", i+1, p.Title, typeStyle.Render(p.Type))
			}
			if len(programs) > previewCount {
				fmt.Printf("       ... and %d more\n", len(programs)-previewCount)
			}
		}
		fmt.Println()
	}

	fmt.Println(headerStyle.Render(strings.Repeat("─", 60)))
	fmt.Printf("%s would schedule %s across %s\n",
		warnStyle.Render("Summary:"),
		successStyle.Render(fmt.Sprintf("%d programs", totalPrograms)),
		successStyle.Render(fmt.Sprintf("%d channel(s)", len(plan))))
	fmt.Println()
	fmt.Println(infoStyle.Render("💡 Use --apply (without --dry-run) to actually apply these changes"))
	fmt.Println()
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
