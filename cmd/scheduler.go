package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/cueconfig"
	"github.com/geekxflood/schedularr/internal/external/tunarr"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	Short: "Manage scheduler configuration files",
	Long:  `Commands for creating, validating, and managing scheduler configuration files.`,
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
		fmt.Printf("\nEdit this file to configure your scheduling blocks, then use:\n")
		fmt.Printf("  schedularr generate --scheduler %s\n", filename)
	},
}

var schedulerValidateCmd = &cobra.Command{
	Use:   "validate [filename]",
	Short: "Validate a scheduler configuration file",
	Long: `Check a scheduler configuration file for syntax errors, invalid cron expressions,
and other configuration issues.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		var filename string
		if len(args) > 0 {
			filename = args[0]
		} else {
			// Try to find default scheduler file
			filename = findDefaultSchedulerFile()
			if filename == "" {
				fmt.Println("Error: No scheduler file found. Specify a file or create scheduler.yaml")
				os.Exit(1)
			}
			fmt.Printf("Validating: %s\n\n", filename)
		}

		// Load and validate the scheduler file
		schedCfg, err := loadSchedulerFile(filename)
		if err != nil {
			fmt.Printf("❌ Validation failed: %v\n", err)
			os.Exit(1)
		}

		// Perform detailed validation
		errors := validateSchedulerConfig(schedCfg)
		if schedulerValidateCheckChannels {
			channelErrors := validateChannelIDs(schedCfg)
			errors = append(errors, channelErrors...)
		}
		if len(errors) > 0 {
			fmt.Printf("❌ Found %d validation error(s):\n\n", len(errors))
			for i, err := range errors {
				fmt.Printf("%d. %v\n", i+1, err)
			}
			os.Exit(1)
		}

		fmt.Printf("✅ Scheduler configuration is valid\n")
		fmt.Printf("   - %d block(s) configured\n", len(schedCfg.Blocks))
		fmt.Printf("   - All cron expressions are valid\n")
		fmt.Printf("   - All required fields present\n")
	},
}

var schedulerListCmd = &cobra.Command{
	Use:   "list [filename]",
	Short: "List all scheduling blocks",
	Long:  `Display all configured scheduling blocks with their details.`,
	Args:  cobra.MaximumNArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		var filename string
		if len(args) > 0 {
			filename = args[0]
		} else {
			filename = findDefaultSchedulerFile()
			if filename == "" {
				fmt.Println("Error: No scheduler file found. Specify a file or create scheduler.yaml")
				os.Exit(1)
			}
		}

		schedCfg, err := loadSchedulerFile(filename)
		if err != nil {
			fmt.Printf("Error loading scheduler file: %v\n", err)
			os.Exit(1)
		}

		if len(schedCfg.Blocks) == 0 {
			fmt.Println("No scheduling blocks configured")
			return
		}

		fmt.Printf("Scheduler configuration: %s\n\n", filename)

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "NAME\tCRON\tDURATION\tCHANNEL\tPRIORITY\tFILTERS")
		_, _ = fmt.Fprintln(w, "----\t----\t--------\t-------\t--------\t-------")

		visibleCount := 0
		for _, block := range schedCfg.Blocks {
			// Apply filters
			if filterChannel != "" && block.ChannelID != filterChannel {
				continue
			}
			if filterPriority != -1 && block.Priority != filterPriority {
				continue
			}

			visibleCount++
			filters := buildFilterSummary(&block.Filter)
			fmt.Fprintf(w, "%s\t%s\t%dm\t%s\t%d\t%s\n",
				block.Name,
				block.Cron,
				block.Duration,
				block.ChannelID,
				block.Priority,
				filters,
			)
		}
		_ = w.Flush()

		fmt.Printf("\nTotal blocks: %d", len(schedCfg.Blocks))
		if visibleCount != len(schedCfg.Blocks) {
			fmt.Printf(" (showing %d)\n", visibleCount)
		} else {
			fmt.Println()
		}
	},
}

var templateType string
var schedulerValidateCheckChannels bool
var filterChannel string
var filterPriority int

func init() {
	rootCmd.AddCommand(schedulerCmd)

	schedulerCmd.AddCommand(schedulerInitCmd)
	schedulerCmd.AddCommand(schedulerValidateCmd)
	schedulerCmd.AddCommand(schedulerListCmd)

	schedulerInitCmd.Flags().StringVarP(&templateType, "template", "t", "basic", "Template type: basic, advanced, or series")
	schedulerValidateCmd.Flags().BoolVar(&schedulerValidateCheckChannels, "check-channels", false, "validate channel IDs against Tunarr")

	schedulerListCmd.Flags().StringVar(&filterChannel, "channel", "", "Filter by channel ID")
	schedulerListCmd.Flags().IntVar(&filterPriority, "priority", -1, "Filter by priority (exact match)")

	// Flags for scheduler init command
	schedulerInitCmd.Flags().StringVar(&blockName, "name", "", "Name for the initial scheduling block")
	schedulerInitCmd.Flags().StringVar(&blockCron, "cron", "", "Cron expression for the initial scheduling block")
	schedulerInitCmd.Flags().IntVar(&blockDuration, "duration", 0, "Duration in minutes for the initial scheduling block")
	schedulerInitCmd.Flags().StringVar(&blockChannelID, "channel-id", "", "Channel ID for the initial scheduling block")
	schedulerInitCmd.Flags().IntVar(&blockPriority, "priority", 0, "Priority for the initial scheduling block")
}

// findDefaultSchedulerFile searches for scheduler.yaml in common locations
func findDefaultSchedulerFile() string {
	searchPaths := []string{
		"scheduler.yaml",
		"./scheduler.yaml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, "scheduler.yaml"))
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// loadSchedulerFile loads and parses a scheduler configuration file
func loadSchedulerFile(filename string) (*scheduler.Config, error) {
	// #nosec G304 - User-specified scheduler file path is intentional
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var cfg scheduler.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &cfg, nil
}

// validateSchedulerConfig performs comprehensive validation on scheduler config
func validateSchedulerConfig(cfg *scheduler.Config) []error {
	if len(cfg.Blocks) == 0 {
		return []error{errors.New("no scheduling blocks defined")}
	}

	var appCfg config.Config
	if err := viper.Unmarshal(&appCfg); err != nil {
		return []error{fmt.Errorf("failed to load app config for cron parser: %w", err)}
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

	var errs []error
	for i, block := range cfg.Blocks {
		errs = append(errs, validateBlock(block, i, parser)...)
	}

	return errs
}

func validateBlock(block scheduler.Block, index int, parser cron.Parser) []error {
	blockPrefix := fmt.Sprintf("Block %d (%s)", index+1, block.Name)

	errs := make([]error, 0, 10) // Preallocate with reasonable capacity
	errs = append(errs, validateBlockRequiredFields(block, blockPrefix, parser)...)
	errs = append(errs, validateBlockFilter(block.Filter, blockPrefix)...)

	return errs
}

func validateBlockRequiredFields(block scheduler.Block, blockPrefix string, parser cron.Parser) []error {
	var errs []error
	if block.Name == "" {
		errs = append(errs, fmt.Errorf("%s: name is required", blockPrefix))
	}

	if block.Cron == "" {
		errs = append(errs, fmt.Errorf("%s: cron expression is required", blockPrefix))
	} else if _, err := parser.Parse(block.Cron); err != nil {
		errs = append(errs, fmt.Errorf("%s: invalid cron expression '%s': %w", blockPrefix, block.Cron, err))
	}

	if block.Duration <= 0 {
		errs = append(errs, fmt.Errorf("%s: duration must be greater than 0", blockPrefix))
	}

	if block.ChannelID == "" {
		errs = append(errs, fmt.Errorf("%s: channel_id is required", blockPrefix))
	}

	return errs
}

func validateBlockFilter(filter scheduler.Filter, blockPrefix string) []error {
	var errs []error
	if filter.MinDuration > 0 && filter.MaxDuration > 0 && filter.MinDuration > filter.MaxDuration {
		errs = append(errs, fmt.Errorf("%s: min_duration (%d) cannot be greater than max_duration (%d)",
			blockPrefix, filter.MinDuration, filter.MaxDuration))
	}

	if filter.YearFrom > 0 && filter.YearTo > 0 && filter.YearFrom > filter.YearTo {
		errs = append(errs, fmt.Errorf("%s: year_from (%d) cannot be greater than year_to (%d)",
			blockPrefix, filter.YearFrom, filter.YearTo))
	}

	return errs
}

func validateChannelIDs(cfg *scheduler.Config) []error {
	var appCfg config.Config
	if err := viper.Unmarshal(&appCfg); err != nil {
		return []error{fmt.Errorf("failed to load app config for channel validation: %w", err)}
	}
	if appCfg.Tunarr.URL == "" {
		return []error{errors.New("tunarr.url is required for channel validation")}
	}

	client := tunarr.NewClient(appCfg.Tunarr)
	channels, err := client.GetChannels(context.Background())
	if err != nil {
		return []error{fmt.Errorf("failed to fetch channels: %w", err)}
	}

	channelIDs := make(map[string]struct{}, len(channels))
	for _, ch := range channels {
		channelIDs[ch.ID] = struct{}{}
	}

	var errs []error
	for i, block := range cfg.Blocks {
		if block.ChannelID == "" {
			continue
		}
		if _, ok := channelIDs[block.ChannelID]; !ok {
			errs = append(errs, fmt.Errorf("block %d (%s): channel_id %q not found in Tunarr", i+1, block.Name, block.ChannelID))
		}
	}

	return errs
}

// buildFilterSummary creates a human-readable summary of filter criteria
func buildFilterSummary(filter *scheduler.Filter) string {
	var parts []string
	parts = appendCountSummary(parts, "genres", filter.Genres)
	parts = appendCountSummary(parts, "ratings", filter.Ratings)

	if yearSummary := buildYearSummary(filter.YearFrom, filter.YearTo); yearSummary != "" {
		parts = append(parts, yearSummary)
	}
	if durationSummary := buildDurationSummary(filter.MinDuration, filter.MaxDuration); durationSummary != "" {
		parts = append(parts, durationSummary)
	}

	if filter.TitlePattern != "" {
		parts = append(parts, "title:regex")
	}

	parts = appendCountSummary(parts, "tags", filter.Tags)

	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, ", ")
}

func appendCountSummary(parts []string, label string, values []string) []string {
	if len(values) == 0 {
		return parts
	}
	return append(parts, fmt.Sprintf("%s:%d", label, len(values)))
}

func buildYearSummary(from int, to int) string {
	switch {
	case from > 0 && to > 0:
		return fmt.Sprintf("year:%d-%d", from, to)
	case from > 0:
		return fmt.Sprintf("year:%d+", from)
	case to > 0:
		return fmt.Sprintf("year:-%d", to)
	default:
		return ""
	}
}

func buildDurationSummary(minMinutes int, maxMinutes int) string {
	switch {
	case minMinutes > 0 && maxMinutes > 0:
		return fmt.Sprintf("dur:%d-%dm", minMinutes, maxMinutes)
	case minMinutes > 0:
		return fmt.Sprintf("dur:%d+m", minMinutes)
	case maxMinutes > 0:
		return fmt.Sprintf("dur:-%dm", maxMinutes)
	default:
		return ""
	}
}
