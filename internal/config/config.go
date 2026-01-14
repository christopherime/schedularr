// Package config provides configuration management for Schedularr.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/geekxflood/schedularr/internal/external/jellyfin"
	"github.com/geekxflood/schedularr/internal/external/radarr"
	"github.com/geekxflood/schedularr/internal/external/sonarr"
	"github.com/geekxflood/schedularr/internal/external/tunarr"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config holds the application configuration
type Config struct {
	Tunarr        tunarr.Config     `mapstructure:"tunarr" yaml:"tunarr" json:"tunarr"`
	Radarr        radarr.Config     `mapstructure:"radarr" yaml:"radarr" json:"radarr,omitempty"`
	Sonarr        sonarr.Config     `mapstructure:"sonarr" yaml:"sonarr" json:"sonarr,omitempty"`
	Jellyfin      jellyfin.Config   `mapstructure:"jellyfin" yaml:"jellyfin" json:"jellyfin,omitempty"`
	Log           LogConfig         `mapstructure:"log" yaml:"log" json:"log"`
	MetricsPort   int               `mapstructure:"metrics_port" yaml:"metrics_port" json:"metrics_port,omitempty"`       // Port for Prometheus metrics endpoint
	Database      string            `mapstructure:"database" yaml:"database" json:"database,omitempty"`                   // Path to SQLite database file
	SchedulerFile string            `mapstructure:"scheduler_file" yaml:"scheduler_file" json:"scheduler_file,omitempty"` // Path to scheduler config file
	Scheduler     scheduler.Config  `mapstructure:"scheduler" yaml:"scheduler" json:"scheduler,omitempty"`                // Inline scheduler config (legacy)
	Cache         CacheConfig       `mapstructure:"cache" yaml:"cache" json:"cache"`
	Maintenance   MaintenanceConfig `mapstructure:"maintenance" yaml:"maintenance" json:"maintenance"`
}

// LogConfig holds configuration for logging
type LogConfig struct {
	Level    string `mapstructure:"level" yaml:"level" json:"level"`
	Format   string `mapstructure:"format" yaml:"format" json:"format"`
	Timezone string `mapstructure:"timezone" yaml:"timezone" json:"timezone,omitempty"` // IANA Time Zone name
}

// CacheConfig holds configuration for content caching
type CacheConfig struct {
	CacheDir      string `mapstructure:"cache_dir" yaml:"cache_dir" json:"cache_dir,omitempty"`                // Directory to store cache files
	CacheDuration string `mapstructure:"cache_duration" yaml:"cache_duration" json:"cache_duration,omitempty"` // How long cache entries are valid (e.g., "1h", "24h")
}

// MaintenanceConfig holds configuration for background maintenance tasks
type MaintenanceConfig struct {
	CleanupInterval  string `mapstructure:"cleanup_interval" yaml:"cleanup_interval" json:"cleanup_interval,omitempty"`    // How often to run cleanup (e.g., "24h")
	HistoryRetention string `mapstructure:"history_retention" yaml:"history_retention" json:"history_retention,omitempty"` // How long to keep schedule history (e.g., "168h" for 7 days)
	CleanupEnabled   bool   `mapstructure:"cleanup_enabled" yaml:"cleanup_enabled" json:"cleanup_enabled"`                 // Whether to enable automatic cleanup
}

// GetCleanupInterval parses the CleanupInterval string into a time.Duration.
// Returns a default of 24 hours if parsing fails.
func (m *MaintenanceConfig) GetCleanupInterval() time.Duration {
	if m.CleanupInterval == "" {
		return 24 * time.Hour
	}
	duration, err := time.ParseDuration(m.CleanupInterval)
	if err != nil {
		return 24 * time.Hour
	}
	return duration
}

// GetHistoryRetention parses the HistoryRetention string into a time.Duration.
// Returns a default of 7 days if parsing fails.
func (m *MaintenanceConfig) GetHistoryRetention() time.Duration {
	if m.HistoryRetention == "" {
		return 7 * 24 * time.Hour // 7 days
	}
	duration, err := time.ParseDuration(m.HistoryRetention)
	if err != nil {
		return 7 * 24 * time.Hour
	}
	return duration
}

// GetCacheDuration parses the CacheDuration string into a time.Duration.
// Returns a default of 1 hour if parsing fails.
func (c *Config) GetCacheDuration() time.Duration {
	duration, err := time.ParseDuration(c.Cache.CacheDuration)
	if err != nil {
		// Log this error properly once a logger is available
		fmt.Fprintf(os.Stderr, "Warning: failed to parse cache duration '%s': %v. Using default 1h.\n", c.Cache.CacheDuration, err)
		return 1 * time.Hour // Default to 1 hour
	}
	return duration
}

// New creates a new Config with default values
func New() *Config {
	return &Config{
		Tunarr: tunarr.Config{
			URL: "http://localhost:8000",
		},
		Radarr: radarr.Config{
			ExcludeMissingFile: true,
		},
		Sonarr: sonarr.Config{
			ExcludeMissingFile: true,
		},
		Jellyfin: jellyfin.Config{},
		Log: LogConfig{
			Level:    "info",
			Format:   "text",
			Timezone: "Local", // Default to system's local timezone
		},
		MetricsPort:   9090, // Default metrics port
		SchedulerFile: "",   // Default to inline config or discover scheduler.yaml
		Cache: CacheConfig{
			CacheDir:      filepath.Join(os.TempDir(), "schedularr_cache"),
			CacheDuration: "1h",
		},
		Maintenance: MaintenanceConfig{
			CleanupInterval:  "24h",
			HistoryRetention: "168h", // 7 days
			CleanupEnabled:   true,
		},
	}
}

// LoadSchedulerConfig loads scheduler configuration from a file or inline config.
// Priority: 1) schedulerFile parameter, 2) Config.SchedulerFile, 3) inline Config.Scheduler, 4) default scheduler.yaml
func LoadSchedulerConfig(cfg *Config, schedulerFile string) (*scheduler.Config, error) {
	if schedulerFile != "" {
		return loadSchedulerFromFile(schedulerFile)
	}

	if cfg.SchedulerFile != "" {
		return loadSchedulerFromFile(cfg.SchedulerFile)
	}

	schedCfg, err := loadSchedulerFromDefaultLocations()
	if err == nil {
		return schedCfg, nil
	}
	if !errors.Is(err, errSchedulerConfigNotFound) {
		return nil, err
	}

	if len(cfg.Scheduler.Blocks) > 0 {
		return &cfg.Scheduler, nil
	}

	return nil, errors.New("no scheduler configuration found")
}

var errSchedulerConfigNotFound = errors.New("no scheduler configuration found in default locations")

func loadSchedulerFromDefaultLocations() (*scheduler.Config, error) {
	searchPaths := []string{
		"scheduler.yaml",
		"./scheduler.yaml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, "scheduler.yaml"))
	}

	for _, path := range searchPaths {
		if _, statErr := os.Stat(path); statErr == nil {
			return loadSchedulerFromFile(path)
		}
	}

	return nil, errSchedulerConfigNotFound
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

// SaveSchedulerConfig saves the scheduler configuration to a YAML file.
// If path is empty, it saves to the default scheduler.yaml in the current directory.
func SaveSchedulerConfig(schedCfg *scheduler.Config, path string) error {
	if path == "" {
		path = "scheduler.yaml"
	}

	data, err := yaml.Marshal(schedCfg)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduler config: %w", err)
	}

	// #nosec G306 - Config file should be readable by owner
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write scheduler file %s: %w", path, err)
	}

	return nil
}
