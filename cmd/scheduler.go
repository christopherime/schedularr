package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/geekxflood/schedularr/internal/config"
	"github.com/geekxflood/schedularr/internal/cueconfig"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
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
extracted from the CUE schema.`,
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

		for _, block := range schedCfg.Blocks {
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

		fmt.Printf("\nTotal blocks: %d\n", len(schedCfg.Blocks))
	},
}

var templateType string
var schedulerValidateCheckChannels bool

func init() {
	rootCmd.AddCommand(schedulerCmd)

	schedulerCmd.AddCommand(schedulerInitCmd)
	schedulerCmd.AddCommand(schedulerValidateCmd)
	schedulerCmd.AddCommand(schedulerListCmd)

	schedulerInitCmd.Flags().StringVarP(&templateType, "template", "t", "basic", "Template type: basic, advanced, or series")
	schedulerValidateCmd.Flags().BoolVar(&schedulerValidateCheckChannels, "check-channels", false, "validate channel IDs against Tunarr")
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

	var errs []error
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
