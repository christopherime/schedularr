// Package config provides configuration management for Schedularr.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/tunarr"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Tunarr        tunarr.Config    `mapstructure:"tunarr" yaml:"tunarr" json:"tunarr"`
	Log           LogConfig        `mapstructure:"log" yaml:"log" json:"log"`
	SchedulerFile string           `mapstructure:"scheduler_file" yaml:"scheduler_file" json:"scheduler_file,omitempty"` // Path to scheduler config file
	Scheduler     scheduler.Config `mapstructure:"scheduler" yaml:"scheduler" json:"scheduler,omitempty"`            // Inline scheduler config (legacy)
}

// LogConfig holds configuration for logging
type LogConfig struct {
	Level  string `mapstructure:"level" yaml:"level" json:"level"`
	Format string `mapstructure:"format" yaml:"format" json:"format"`
}

// New creates a new Config with default values
func New() *Config {
	return &Config{
		Tunarr: tunarr.Config{
			URL: "http://localhost:8000",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
		},
		SchedulerFile: "", // Default to inline config or discover scheduler.yaml
	}
}

// LoadSchedulerConfig loads scheduler configuration from a file or inline config.
// Priority: 1) schedulerFile parameter, 2) Config.SchedulerFile, 3) inline Config.Scheduler, 4) default scheduler.yaml
func LoadSchedulerConfig(cfg *Config, schedulerFile string) (*scheduler.Config, error) {
	var schedCfg *scheduler.Config
	var err error

	// Priority 1: Explicit scheduler file parameter
	if schedulerFile != "" {
		schedCfg, err = loadSchedulerFromFile(schedulerFile)
	} else if cfg.SchedulerFile != "" {
		// Priority 2: SchedulerFile field in config
		schedCfg, err = loadSchedulerFromFile(cfg.SchedulerFile)
	} else {
		// Priority 3: Check for default scheduler.yaml in multiple locations
		found := false
		searchPaths := []string{
			"scheduler.yaml",
			"./scheduler.yaml",
		}

		// Add home directory path
		if home, err := os.UserHomeDir(); err == nil {
			searchPaths = append(searchPaths, filepath.Join(home, "scheduler.yaml"))
		}

		for _, path := range searchPaths {
			if _, err := os.Stat(path); err == nil {
				schedCfg, err = loadSchedulerFromFile(path)
				found = true
				break
			}
		}

		if !found {
			// Priority 4: Use inline scheduler config (legacy support)
			if len(cfg.Scheduler.Blocks) > 0 {
				schedCfg = &cfg.Scheduler
			} else {
				return nil, errors.New("no scheduler configuration found")
			}
		}
	}

	if err != nil {
		return nil, err
	}

	// Validate configuration
	if err := ValidateSchedulerConfigStruct(schedCfg); err != nil {
		return nil, fmt.Errorf("scheduler config validation failed: %w", err)
	}

	return schedCfg, nil
}

// loadSchedulerFromFile loads scheduler config from a YAML file
func loadSchedulerFromFile(path string) (*scheduler.Config, error) {
	// #nosec G304 - User-specified config file path is intentional
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read scheduler file %s: %w", path, err)
	}

	var schedCfg scheduler.Config
	if err := yaml.Unmarshal(data, &schedCfg); err != nil {
		return nil, fmt.Errorf("failed to parse scheduler file %s: %w", path, err)
	}

	return &schedCfg, nil
}

// GetSchedulerConfig is a helper that loads scheduler config using Viper
func GetSchedulerConfig(schedulerFile string) (*scheduler.Config, error) {
	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return LoadSchedulerConfig(&cfg, schedulerFile)
}
