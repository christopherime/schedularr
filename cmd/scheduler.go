package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/geekxflood/schedularr/internal/cueconfig"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/robfig/cron/v3"
	"github.com/spf13/cobra"
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

func init() {
	rootCmd.AddCommand(schedulerCmd)

	schedulerCmd.AddCommand(schedulerInitCmd)
	schedulerCmd.AddCommand(schedulerValidateCmd)
	schedulerCmd.AddCommand(schedulerListCmd)

	schedulerInitCmd.Flags().StringVarP(&templateType, "template", "t", "basic", "Template type: basic, advanced, or series")
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
	var errs []error
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

	if len(cfg.Blocks) == 0 {
		errs = append(errs, errors.New("no scheduling blocks defined"))
		return errs
	}

	for i, block := range cfg.Blocks {
		blockPrefix := fmt.Sprintf("Block %d (%s)", i+1, block.Name)

		// Validate required fields
		if block.Name == "" {
			errs = append(errs, fmt.Errorf("%s: name is required", blockPrefix))
		}

		if block.Cron == "" {
			errs = append(errs, fmt.Errorf("%s: cron expression is required", blockPrefix))
		} else {
			// Validate cron expression
			if _, err := parser.Parse(block.Cron); err != nil {
				errs = append(errs, fmt.Errorf("%s: invalid cron expression '%s': %w", blockPrefix, block.Cron, err))
			}
		}

		if block.Duration <= 0 {
			errs = append(errs, fmt.Errorf("%s: duration must be greater than 0", blockPrefix))
		}

		if block.ChannelID == "" {
			errs = append(errs, fmt.Errorf("%s: channel_id is required", blockPrefix))
		}

		// Validate filter constraints
		if block.Filter.MinDuration > 0 && block.Filter.MaxDuration > 0 {
			if block.Filter.MinDuration > block.Filter.MaxDuration {
				errs = append(errs, fmt.Errorf("%s: min_duration (%d) cannot be greater than max_duration (%d)",
					blockPrefix, block.Filter.MinDuration, block.Filter.MaxDuration))
			}
		}

		if block.Filter.YearFrom > 0 && block.Filter.YearTo > 0 {
			if block.Filter.YearFrom > block.Filter.YearTo {
				errs = append(errs, fmt.Errorf("%s: year_from (%d) cannot be greater than year_to (%d)",
					blockPrefix, block.Filter.YearFrom, block.Filter.YearTo))
			}
		}
	}

	return errs
}

// buildFilterSummary creates a human-readable summary of filter criteria
func buildFilterSummary(filter *scheduler.Filter) string {
	var parts []string

	if len(filter.Genres) > 0 {
		parts = append(parts, fmt.Sprintf("genres:%d", len(filter.Genres)))
	}

	if len(filter.Ratings) > 0 {
		parts = append(parts, fmt.Sprintf("ratings:%d", len(filter.Ratings)))
	}

	if filter.YearFrom > 0 || filter.YearTo > 0 {
		if filter.YearFrom > 0 && filter.YearTo > 0 {
			parts = append(parts, fmt.Sprintf("year:%d-%d", filter.YearFrom, filter.YearTo))
		} else if filter.YearFrom > 0 {
			parts = append(parts, fmt.Sprintf("year:%d+", filter.YearFrom))
		} else {
			parts = append(parts, fmt.Sprintf("year:-%d", filter.YearTo))
		}
	}

	if filter.MinDuration > 0 || filter.MaxDuration > 0 {
		if filter.MinDuration > 0 && filter.MaxDuration > 0 {
			parts = append(parts, fmt.Sprintf("dur:%d-%dm", filter.MinDuration, filter.MaxDuration))
		} else if filter.MinDuration > 0 {
			parts = append(parts, fmt.Sprintf("dur:%d+m", filter.MinDuration))
		} else {
			parts = append(parts, fmt.Sprintf("dur:-%dm", filter.MaxDuration))
		}
	}

	if filter.TitlePattern != "" {
		parts = append(parts, "title:regex")
	}

	if len(filter.Tags) > 0 {
		parts = append(parts, fmt.Sprintf("tags:%d", len(filter.Tags)))
	}

	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, ", ")
}
