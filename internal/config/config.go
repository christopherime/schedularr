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
	Tunarr        tunarr.Config    `mapstructure:"tunarr" yaml:"tunarr"`
	Log           LogConfig        `mapstructure:"log" yaml:"log"`
	SchedulerFile string           `mapstructure:"scheduler_file" yaml:"scheduler_file"` // Path to scheduler config file
	Scheduler     scheduler.Config `mapstructure:"scheduler" yaml:"scheduler"`            // Inline scheduler config (legacy)
}

// LogConfig holds configuration for logging
type LogConfig struct {
	Level  string `mapstructure:"level" yaml:"level"`
	Format string `mapstructure:"format" yaml:"format"`
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
	// Priority 1: Explicit scheduler file parameter
	if schedulerFile != "" {
		return loadSchedulerFromFile(schedulerFile)
	}

	// Priority 2: SchedulerFile field in config
	if cfg.SchedulerFile != "" {
		return loadSchedulerFromFile(cfg.SchedulerFile)
	}

	// Priority 3: Check for default scheduler.yaml in multiple locations
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
			return loadSchedulerFromFile(path)
		}
	}

	// Priority 4: Use inline scheduler config (legacy support)
	if len(cfg.Scheduler.Blocks) > 0 {
		return &cfg.Scheduler, nil
	}

	return nil, errors.New("no scheduler configuration found")
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
