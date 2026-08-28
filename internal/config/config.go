// Package config provides configuration loading for Schedularr.
// Configuration structure and defaults are defined by CUE schemas in cmd/schema/.
// This package re-exports cueconfig types and provides loading helpers.
package config

import (
	"time"

	"github.com/christopherime/schedularr/internal/cueconfig"
	"github.com/christopherime/schedularr/internal/external/tunarr"
)

// Config is an alias for cueconfig.Config
// All config access should use the accessor methods (GetString, GetInt, etc.)
type Config = cueconfig.Config

// Load loads configuration from a file using CUE validation and defaults.
func Load(path string) (*Config, error) {
	validator := cueconfig.NewValidator()
	return validator.LoadConfigWithEnvInterpolation(path)
}

// SchedulerFilePath returns the configured path of the scheduler.yaml
// import file (see blockio.Bootstrap). scheduler.yaml is import-only: it
// seeds the block store on first run and is otherwise ignored, so this is
// only ever consulted for that one-time bootstrap.
func SchedulerFilePath(cfg *Config) string {
	return cfg.GetString("scheduler_file")
}

// ============================================================================
// Client Configuration Helpers
// Extract typed client configs from CUE-loaded configuration
// ============================================================================

// TunarrConfig extracts Tunarr client configuration from the loaded config.
func TunarrConfig(cfg *Config) tunarr.Config {
	m := cfg.GetMap("tunarr")
	return tunarr.Config{
		URL:    getStringFromMap(m, "url"),
		APIKey: getStringFromMap(m, "api_key"),
	}
}

// LogLevel returns the configured log level.
func LogLevel(cfg *Config) string {
	return cfg.GetString("log.level")
}

// LogFormat returns the configured log format.
func LogFormat(cfg *Config) string {
	return cfg.GetString("log.format")
}

// LogTimezone returns the configured timezone.
func LogTimezone(cfg *Config) string {
	return cfg.GetString("log.timezone")
}

// MetricsPort returns the configured metrics port.
func MetricsPort(cfg *Config) int {
	return cfg.GetInt("metrics_port")
}

// DatabasePath returns the configured database path.
func DatabasePath(cfg *Config) string {
	return cfg.GetString("database")
}

// CacheDuration returns the configured cache duration.
func CacheDuration(cfg *Config) time.Duration {
	return cfg.GetDuration("cache.cache_duration")
}

// MaintenanceCleanupEnabled returns whether cleanup is enabled.
func MaintenanceCleanupEnabled(cfg *Config) bool {
	return cfg.GetBool("maintenance.cleanup_enabled")
}

// MaintenanceHistoryRetention returns the history retention duration.
func MaintenanceHistoryRetention(cfg *Config) time.Duration {
	return cfg.GetDuration("maintenance.history_retention")
}

// ============================================================================
// Helper Functions
// ============================================================================

func getStringFromMap(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
