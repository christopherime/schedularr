// Package config provides configuration loading for Schedularr.
// Configuration structure and defaults are defined by CUE schemas in cmd/schema/.
// This package re-exports cueconfig types and provides loading helpers.
package config

import (
	"os"
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

// CronInterval returns the configured interval between the `serve`
// command's cron-loop schedule regenerate-and-apply cycles, CUE-defaulting
// to 6h when the config file doesn't set cron_interval. This is a
// top-level key, not under api.*, because it governs the scheduler's cron
// behavior, not the HTTP server -- `serve --interval`/`-i` overrides this
// value when passed explicitly (flag > config > 6h default).
func CronInterval(cfg *Config) time.Duration {
	return cfg.GetDuration("cron_interval")
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

// DatabasePath returns the configured database path.
func DatabasePath(cfg *Config) string {
	return cfg.GetString("database")
}

// MaintenanceCleanupEnabled returns whether cleanup is enabled.
func MaintenanceCleanupEnabled(cfg *Config) bool {
	return cfg.GetBool("maintenance.cleanup_enabled")
}

// MaintenanceHistoryRetention returns the history retention duration.
func MaintenanceHistoryRetention(cfg *Config) time.Duration {
	return cfg.GetDuration("maintenance.history_retention")
}

// APIListen returns the configured address for the `serve` command's HTTP
// API server to listen on (host:port, or :port for all interfaces).
func APIListen(cfg *Config) string {
	return cfg.GetString("api.listen")
}

// APIToken returns the bearer token the `serve` command's API server
// requires on /api/v1/* requests. The SCHEDULARR_API_TOKEN environment
// variable always wins over the api.token config key: operators rotate a
// deployed token via the environment (a systemd unit's Environment=, a
// Docker secret, ...) independently of whatever's checked into the config
// file, and an env-var override is easier to keep out of version control
// than a config value.
func APIToken(cfg *Config) string {
	if v := os.Getenv(EnvAPIToken); v != "" {
		return v
	}
	return cfg.GetString("api.token")
}

// APIInsecureNoAuth returns whether the `serve` command's API server should
// skip bearer-auth on /api/v1/* entirely (api.Config.InsecureNoAuth).
// Local development only.
func APIInsecureNoAuth(cfg *Config) bool {
	return cfg.GetBool("api.insecure_no_auth")
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
