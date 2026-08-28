package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/christopherime/schedularr/internal/blockio"
	"github.com/christopherime/schedularr/internal/cache"
	"github.com/christopherime/schedularr/internal/config"
	"github.com/christopherime/schedularr/internal/cueconfig"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/spf13/cobra"
)

var (
	apply     bool
	assumeYes bool
	dryRun    bool
	verbose   bool

	// Flags for generate config subcommand
	configOutputPath string
	genTunarrURL     string
	genTunarrAPIKey  string
	genLogLevel      string
	genLogFormat     string
)

// colorEnabled reports whether ANSI color codes should be emitted. It mirrors
// the auto-detection lipgloss used to do: honor NO_COLOR
// (https://no-color.org/) and only colorize when stdout is an actual
// terminal, so piped, redirected, cron, and CI output (this task's own
// --yes-driven scripting use case included) stays escape-code free.
// Evaluated lazily (not cached) so it always reflects the current
// environment/redirection state, which also keeps it directly testable.
func colorEnabled() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// style is a minimal ANSI text-color helper for CLI output. It replaces the
// former charmbracelet/lipgloss dependency: the TUI (lipgloss's only other
// consumer) is gone, so this package keeps colored terminal output without
// pulling in the library.
type style struct {
	code string
}

// Render wraps str in the style's ANSI escape sequence, or returns it
// unstyled when color output is disabled (see colorEnabled).
func (s style) Render(str string) string {
	if !colorEnabled() {
		return str
	}
	return "\x1b[" + s.code + "m" + str + "\x1b[0m"
}

// Color styles
var (
	successStyle = style{code: "1;32"} // Bold green
	errorStyle   = style{code: "1;31"} // Bold red
	infoStyle    = style{code: "36"}   // Cyan
	warnStyle    = style{code: "33"}   // Yellow

	// Program type colors
	movieStyle   = style{code: "35"}   // Magenta
	episodeStyle = style{code: "34"}   // Blue
	trackStyle   = style{code: "36"}   // Cyan
	headerStyle  = style{code: "1;37"} // White/Bold
	channelStyle = style{code: "1;32"} // Green/Bold
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate schedule or configuration files",
	Long: `Generate programming schedules or configuration files.

Subcommands:
  config    Generate a configuration file from CUE schema

Without a subcommand, generates programming schedules based on configured blocks.

Schedule generation:
1. Fetches available content from Tunarr libraries
2. Applies filtering rules from each block
3. Generates optimized schedules with conflict resolution
4. Optionally applies the schedule to Tunarr channels

Use --dry-run to preview schedules without applying them.
Use --verbose for detailed output including filtering and history.`,
	Run: func(_ *cobra.Command, _ []string) {
		cfg := getConfig()
		if cfg == nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s config not loaded\n", errorStyle.Render("✗ Error:"))
			os.Exit(1)
		}

		if err := ProcessSchedule(cfg, apply, dryRun); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error:"), err)
			os.Exit(1)
		}
	},
}

var generateConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Generate application configuration file from CUE schema",
	Long: `Generate an application configuration file from the embedded CUE schema with defaults.

The generated file will contain all configuration options with their default values
extracted from the CUE schema. Output format (YAML/JSON) is determined by file extension.

You can override default values using flags.

Examples:
  schedularr generate config --output config.yaml
  schedularr generate config --output my-config.json --tunarr-url "http://my-tunarr:8000"
  schedularr generate config -o config.yaml --log-level debug`,
	Run: func(_ *cobra.Command, _ []string) {
		if configOutputPath == "" {
			_, _ = fmt.Fprintf(os.Stderr, "%s --output flag is required\n", errorStyle.Render("✗ Error:"))
			os.Exit(1)
		}

		// Check if file already exists
		if _, err := os.Stat(configOutputPath); err == nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s File %s already exists. Remove it first or choose a different name.\n",
				errorStyle.Render("✗ Error:"), configOutputPath)
			os.Exit(1)
		}

		// Determine format from extension
		ext := strings.ToLower(filepath.Ext(configOutputPath))
		format := "yaml"
		if ext == ".json" {
			format = "json"
		}

		// Build overrides map from flags (only include non-empty values)
		overrides := buildConfigOverrides()

		// Generate from CUE schema with overrides
		validator := cueconfig.NewValidator()
		data, err := validator.GenerateConfigWithOverrides(format, overrides)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error generating config:"), err)
			os.Exit(1)
		}

		if err := os.WriteFile(configOutputPath, data, 0o600); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%s %v\n", errorStyle.Render("✗ Error creating file:"), err)
			os.Exit(1)
		}

		fmt.Printf("%s %s\n", successStyle.Render("✓ Created configuration file:"), configOutputPath)
		fmt.Printf("Format: %s\n", format)
		fmt.Printf("\nEdit this file to configure Schedularr, then use:\n")
		fmt.Printf("  schedularr --config %s <command>\n", configOutputPath)
	},
}

// buildConfigOverrides constructs a nested map of config overrides from command flags.
func buildConfigOverrides() map[string]interface{} {
	overrides := make(map[string]interface{})

	// Tunarr overrides
	tunarrOverrides := make(map[string]interface{})
	if genTunarrURL != "" {
		tunarrOverrides["url"] = genTunarrURL
	}
	if genTunarrAPIKey != "" {
		tunarrOverrides["api_key"] = genTunarrAPIKey
	}
	if len(tunarrOverrides) > 0 {
		overrides["tunarr"] = tunarrOverrides
	}

	// Log overrides
	logOverrides := make(map[string]interface{})
	if genLogLevel != "" {
		logOverrides["level"] = genLogLevel
	}
	if genLogFormat != "" {
		logOverrides["format"] = genLogFormat
	}
	if len(logOverrides) > 0 {
		overrides["log"] = logOverrides
	}

	return overrides
}

// ProcessSchedule generates and optionally applies the schedule.
func ProcessSchedule(cfg *config.Config, applyFlag bool, dryRunFlag bool) error {
	blocks, st, err := initializeScheduler(cfg)
	if err != nil {
		return err
	}
	defer st.Close()

	client := tunarr.NewClient(config.TunarrConfig(cfg))
	programs, err := fetchAndValidateContent(cfg, client)
	if err != nil {
		return err
	}

	engine, err := createEngine(cfg, client, blocks, st)
	if err != nil {
		return err
	}

	plan, err := generateSchedulePlan(cfg, engine, programs)
	if err != nil {
		return err
	}

	displaySchedule(plan, verbose)
	flattenedPlan := flattenSchedule(plan)

	err = handleScheduleOutput(cfg, client, engine, flattenedPlan, scheduleOutputOptions{
		apply:  applyFlag,
		dryRun: dryRunFlag,
	})
	if err != nil {
		return err
	}

	// Run cleanup after successful apply if enabled
	if applyFlag && !dryRunFlag && config.MaintenanceCleanupEnabled(cfg) {
		runScheduleHistoryCleanup(cfg, st)
	}

	return nil
}

// runScheduleHistoryCleanup removes old schedule history entries based on retention policy.
func runScheduleHistoryCleanup(cfg *config.Config, st *store.Store) {
	retention := config.MaintenanceHistoryRetention(cfg)
	if retention == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	removed, err := st.CleanupScheduleHistory(ctx, retention)
	if err != nil {
		fmt.Printf("%s %v\n", warnStyle.Render("⚠ Schedule history cleanup failed (non-fatal):"), err)
		return
	}

	if removed > 0 {
		fmt.Printf("%s Cleaned up %d old schedule history entries\n", successStyle.Render("✓"), removed)
	}
}

// initializeScheduler opens the store, imports scheduler.yaml into it on
// first run (see blockio.Bootstrap), and returns the currently active
// blocks. scheduler.yaml is import-only from here on: the store is the
// engine's source of truth.
func initializeScheduler(cfg *config.Config) ([]scheduler.Block, *store.Store, error) {
	st, err := store.New(config.DatabasePath(cfg))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	ctx := context.Background()
	logger := newLogger(config.LogLevel(cfg), config.LogFormat(cfg))
	schedFile := config.SchedulerFilePath(cfg)

	imported, err := blockio.Bootstrap(ctx, st, schedFile, logger)
	if err != nil {
		_ = st.Close()
		return nil, nil, fmt.Errorf("failed to bootstrap blocks from %s: %w", schedFile, err)
	}
	if imported > 0 {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📥 Imported %d scheduling block(s) from %s", imported, schedFile)))
	}

	blocks, err := loadActiveBlocks(ctx, st)
	if err != nil {
		_ = st.Close()
		return nil, nil, fmt.Errorf("failed to load scheduling blocks: %w", err)
	}
	if len(blocks) == 0 {
		_ = st.Close()
		return nil, nil, errors.New("no scheduling blocks configured")
	}

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📋 Loaded %d scheduling block(s)", len(blocks))))

	return blocks, st, nil
}

// loadActiveBlocks returns the Spec of every enabled block in the store.
// scheduler.yaml is import-only (see blockio.Bootstrap): the store is the
// engine's live source of scheduling truth, and disabled blocks stay
// defined but out of schedule generation.
func loadActiveBlocks(ctx context.Context, s *store.Store) ([]scheduler.Block, error) {
	records, err := s.ListBlocks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list blocks from store: %w", err)
	}

	blocks := make([]scheduler.Block, 0, len(records))
	for _, rec := range records {
		if !rec.Enabled {
			continue
		}
		blocks = append(blocks, rec.Spec)
	}
	return blocks, nil
}

func fetchAndValidateContent(cfg *config.Config, client *tunarr.Client) ([]tunarr.Program, error) {
	programs, err := fetchAllContent(cfg, client)
	if err != nil {
		return nil, err
	}

	fmt.Printf("%s\n", successStyle.Render(fmt.Sprintf("✓ Found %d program(s)", len(programs))))
	return programs, nil
}

const tunarrCacheKey = "tunarr_programs.json"

// fetchAllContent fetches all schedulable content from Tunarr's libraries,
// caching the result for the configured cache duration. Falls back to
// SearchPrograms() when no library content is available.
func fetchAllContent(cfg *config.Config, tunarrClient *tunarr.Client) ([]tunarr.Program, error) {
	fmt.Println(infoStyle.Render("📡 Fetching content..."))

	cacheDuration := config.CacheDuration(cfg)
	contentCache, err := cache.New(cacheDuration)
	if err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not initialize content cache: %v. Proceeding without cache.", err)))
		contentCache = nil // Disable caching if initialization failed
	} else {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🗄️  Using in-memory cache (duration %s)", cacheDuration)))
	}

	programs := fetchTunarrContent(tunarrClient, contentCache)

	if len(programs) == 0 {
		fmt.Println(warnStyle.Render("⚠ No content available from libraries - using fallback SearchPrograms()"))
		var err error
		programs, err = fetchAllProgramsViaSearch(tunarrClient)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch programs: %w", err)
		}
	}

	return programs, nil
}

func fetchTunarrContent(client *tunarr.Client, contentCache *cache.Cache) []tunarr.Program {
	if programs := tryLoadTunarrFromCache(contentCache); programs != nil {
		return programs
	}

	allPrograms := fetchLibraryPrograms(client)
	saveTunarrCache(contentCache, allPrograms)

	return allPrograms
}

func tryLoadTunarrFromCache(contentCache *cache.Cache) []tunarr.Program {
	if contentCache == nil {
		return nil
	}
	data, found := contentCache.Get(tunarrCacheKey)
	if !found {
		return nil
	}
	programs, ok := data.([]tunarr.Program)
	if !ok {
		return nil
	}
	if verbose {
		fmt.Printf("%s\n", infoStyle.Render("📖 Loaded Tunarr programs from cache"))
	}
	return programs
}

func fetchLibraryPrograms(client *tunarr.Client) []tunarr.Program {
	fmt.Println(infoStyle.Render("📡 Fetching content from Tunarr..."))

	// First, get all media sources
	sources, err := client.GetMediaSources(context.Background())
	if err != nil {
		if verbose {
			fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch media sources: %v", err)))
		}
		return nil
	}

	if verbose {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📡 Found %d media source(s)", len(sources))))
	}

	// Collect libraries from all media sources
	var allLibraries []tunarr.Library
	for _, source := range sources {
		libraries, err := client.GetLibraries(context.Background(), source.ID)
		if err != nil {
			if verbose {
				fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Could not fetch libraries for %s: %v", source.Name, err)))
			}
			continue
		}
		allLibraries = append(allLibraries, libraries...)
	}

	if verbose {
		fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("📚 Found %d librar(y/ies)", len(allLibraries))))
	}

	var allPrograms []tunarr.Program
	for _, lib := range allLibraries {
		programs := fetchSingleLibrary(client, lib)
		allPrograms = append(allPrograms, programs...)
	}

	return allPrograms
}

func fetchSingleLibrary(client *tunarr.Client, lib tunarr.Library) []tunarr.Program {
	if verbose {
		fmt.Printf("  - %s (%s)\n", lib.Name, lib.Type)
	}

	// Use SearchPrograms with library ID to get programs from this library
	var allPrograms []tunarr.Program
	page := 1
	limit := 100

	for {
		req := tunarr.ProgramSearchRequest{
			Query:     &tunarr.ProgramSearchQuery{}, // API requires query object
			LibraryID: lib.ID,
			Page:      page,
			Limit:     limit,
		}

		resp, err := client.SearchPrograms(context.Background(), req)
		if err != nil {
			if verbose {
				fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("    ⚠ Could not fetch programs from %s: %v", lib.Name, err)))
			}
			return nil
		}

		allPrograms = append(allPrograms, resp.Results...)

		// Check if we've fetched all programs
		if len(resp.Results) < limit || len(allPrograms) >= resp.Total {
			break
		}
		page++
	}

	if verbose {
		fmt.Printf("    ✓ %d programs\n", len(allPrograms))
	}

	return allPrograms
}

func fetchAllProgramsViaSearch(client *tunarr.Client) ([]tunarr.Program, error) {
	var allPrograms []tunarr.Program
	page := 1
	limit := 100

	for {
		req := tunarr.ProgramSearchRequest{
			Query: &tunarr.ProgramSearchQuery{}, // API requires query object
			Page:  page,
			Limit: limit,
		}

		resp, err := client.SearchPrograms(context.Background(), req)
		if err != nil {
			return nil, err
		}

		allPrograms = append(allPrograms, resp.Results...)

		// Check if we've fetched all programs
		if len(resp.Results) < limit || len(allPrograms) >= resp.Total {
			break
		}
		page++
	}

	return allPrograms, nil
}

func saveTunarrCache(contentCache *cache.Cache, allPrograms []tunarr.Program) {
	if contentCache == nil {
		return
	}

	if err := contentCache.Set(tunarrCacheKey, allPrograms); err != nil {
		fmt.Printf("%s\n", warnStyle.Render(fmt.Sprintf("⚠ Error writing Tunarr programs to cache: %v", err)))
	}
}

func createEngine(cfg *config.Config, client *tunarr.Client, blocks []scheduler.Block, st *store.Store) (*scheduler.Engine, error) {
	logger := newLogger(config.LogLevel(cfg), config.LogFormat(cfg))

	timezone := config.LogTimezone(cfg)
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone '%s' in app config: %w", timezone, err)
	}

	return scheduler.NewEngine(client, blocks, st, logger, loc), nil
}

func generateSchedulePlan(cfg *config.Config, engine *scheduler.Engine, programs []tunarr.Program) (map[string][]scheduler.ScheduledSlot, error) {
	start := time.Now()
	end := start.Add(24 * time.Hour)

	fmt.Printf("%s\n", infoStyle.Render(fmt.Sprintf("🗓️  Generating schedule from %s to %s in %s",
		start.Format("2006-01-02 15:04"),
		end.Format("2006-01-02 15:04"),
		config.LogTimezone(cfg))))

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
		if !assumeYes {
			return errors.New("refusing to apply without --yes (interactive prompts were removed)")
		}
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

func applyScheduleAndSync(_ *config.Config, client *tunarr.Client, engine *scheduler.Engine, flattenedPlan map[string][]tunarr.Program) error {
	if err := applySchedule(client, flattenedPlan); err != nil {
		return err
	}

	if err := engine.Commit(); err != nil {
		return fmt.Errorf("failed to commit state: %w", err)
	}

	return nil
}

type channelStats struct {
	programCount  int
	totalDuration int64
	movies        int
	episodes      int
	tracks        int
}

func (cs *channelStats) incrementType(programType string) style {
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

func displayChannelSchedule(channelID string, slots []scheduler.ScheduledSlot) channelStats {
	stats := channelStats{}

	fmt.Println("\n" + channelStyle.Render("📺 Channel "+channelID))

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Start", "End", "Block", "Program", "Duration", "Type", "Show", "S", "E"})

	for _, slot := range slots {
		currentTime := slot.StartTime
		for _, program := range slot.Programs {
			programEndTime := currentTime.Add(time.Duration(program.GetDurationMs()) * time.Millisecond)
			typeStyle := stats.incrementType(program.Type)

			t.AppendRow(table.Row{
				currentTime.Format("15:04"),
				programEndTime.Format("15:04"),
				slot.Block.Name,
				program.Title,
				fmtDuration(program.GetDurationMs()),
				typeStyle.Render(program.Type),
				program.ShowTitle,
				program.SeasonNumber,
				program.EpisodeNumber,
			})

			currentTime = programEndTime
			stats.totalDuration += program.GetDurationMs()
			stats.programCount++
		}
	}

	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 4, WidthMax: 30, WidthMaxEnforcer: text.WrapSoft},
		{Number: 7, WidthMax: 20, WidthMaxEnforcer: text.WrapSoft},
	})
	t.SetStyle(table.StyleLight)
	t.Render()
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
			totalDuration += p.GetDurationMs()
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
				var typeStyle style
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

	// Flags for schedule generation (default behavior)
	generateCmd.Flags().BoolVar(&apply, "apply", false, "Apply generated schedule to Tunarr channels")
	generateCmd.Flags().BoolVar(&assumeYes, "yes", false, "Skip confirmation and allow --apply to run non-interactively")
	generateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview schedule without applying changes")
	generateCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show detailed output including filtering and history")

	// Add config subcommand
	generateCmd.AddCommand(generateConfigCmd)

	// Flags for generate config subcommand
	generateConfigCmd.Flags().StringVarP(&configOutputPath, "output", "o", "", "Output file path (required)")
	generateConfigCmd.Flags().StringVar(&genTunarrURL, "tunarr-url", "", "Override default Tunarr API URL")
	generateConfigCmd.Flags().StringVar(&genTunarrAPIKey, "tunarr-api-key", "", "Override default Tunarr API Key")
	generateConfigCmd.Flags().StringVar(&genLogLevel, "log-level", "", "Override default log level (debug, info, warn, error)")
	generateConfigCmd.Flags().StringVar(&genLogFormat, "log-format", "", "Override default log format (text, json)")
	_ = generateConfigCmd.MarkFlagRequired("output")
}
