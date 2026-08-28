// Package config provides configuration loading for Schedularr.
// Configuration structure and defaults are defined by CUE schemas in cmd/schema/.
// This package re-exports cueconfig types and provides loading helpers.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/christopherime/schedularr/internal/cueconfig"
	"github.com/christopherime/schedularr/internal/external/jellyfin"
	"github.com/christopherime/schedularr/internal/external/radarr"
	"github.com/christopherime/schedularr/internal/external/sonarr"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
	"gopkg.in/yaml.v3"
)

// Config is an alias for cueconfig.Config
// All config access should use the accessor methods (GetString, GetInt, etc.)
type Config = cueconfig.Config

// SchedulerConfig is an alias for cueconfig.SchedulerConfig
type SchedulerConfig = cueconfig.SchedulerConfig

// Load loads configuration from a file using CUE validation and defaults.
func Load(path string) (*Config, error) {
	validator := cueconfig.NewValidator()
	return validator.LoadConfigWithEnvInterpolation(path)
}

// LoadScheduler loads scheduler configuration from a file.
func LoadScheduler(path string) (*SchedulerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read scheduler file: %w", err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	validator := cueconfig.NewValidator()
	return validator.LoadSchedulerConfig([]byte(expanded), "yaml")
}

// FindSchedulerConfig finds scheduler configuration from various sources.
// Priority: 1) schedulerFile parameter, 2) cfg.scheduler_file, 3) default locations
func FindSchedulerConfig(cfg *Config, schedulerFile string) (*SchedulerConfig, error) {
	if schedulerFile != "" {
		return LoadScheduler(schedulerFile)
	}

	if cfgSchedulerFile := cfg.GetString("scheduler_file"); cfgSchedulerFile != "" {
		return LoadScheduler(cfgSchedulerFile)
	}

	// Try default locations
	return loadSchedulerFromDefaultLocations()
}

var errSchedulerConfigNotFound = errors.New("no scheduler configuration found in default locations")

func loadSchedulerFromDefaultLocations() (*SchedulerConfig, error) {
	searchPaths := []string{
		"scheduler.yaml",
		"./scheduler.yaml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		searchPaths = append(searchPaths, filepath.Join(home, "scheduler.yaml"))
	}

	for _, path := range searchPaths {
		if _, statErr := os.Stat(path); statErr == nil {
			return LoadScheduler(path)
		}
	}

	return nil, errSchedulerConfigNotFound
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

// JellyfinConfig extracts Jellyfin client configuration from the loaded config.
func JellyfinConfig(cfg *Config) jellyfin.Config {
	m := cfg.GetMap("jellyfin")
	return jellyfin.Config{
		URL:        getStringFromMap(m, "url"),
		APIKey:     getStringFromMap(m, "api_key"),
		UserID:     getStringFromMap(m, "user_id"),
		SyncLiveTV: getBoolFromMap(m, "sync_live_tv"),
	}
}

// RadarrConfig extracts Radarr client configuration from the loaded config.
func RadarrConfig(cfg *Config) radarr.Config {
	m := cfg.GetMap("radarr")
	return radarr.Config{
		URL:                getStringFromMap(m, "url"),
		APIKey:             getStringFromMap(m, "api_key"),
		ExcludeMissingFile: getBoolFromMap(m, "exclude_missing_file"),
	}
}

// SonarrConfig extracts Sonarr client configuration from the loaded config.
func SonarrConfig(cfg *Config) sonarr.Config {
	m := cfg.GetMap("sonarr")
	return sonarr.Config{
		URL:                getStringFromMap(m, "url"),
		APIKey:             getStringFromMap(m, "api_key"),
		ExcludeMissingFile: getBoolFromMap(m, "exclude_missing_file"),
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
// Scheduler Configuration Helpers
// Convert CUE-loaded scheduler config to scheduler.Block types
// ============================================================================

// SchedulerBlocks converts SchedulerConfig blocks to scheduler.Block slice.
func SchedulerBlocks(schedCfg *SchedulerConfig) []scheduler.Block {
	blockMaps := schedCfg.GetBlocks()
	blocks := make([]scheduler.Block, 0, len(blockMaps))
	for _, bm := range blockMaps {
		blocks = append(blocks, blockFromMap(bm))
	}
	return blocks
}

// blockFromMap converts a map to a scheduler.Block
func blockFromMap(m map[string]any) scheduler.Block {
	block := scheduler.Block{
		Type:                       scheduler.BlockType(getStringWithDefault(m, "type", "filter")),
		Name:                       getStringFromMap(m, "name"),
		Cron:                       getStringFromMap(m, "cron"),
		Duration:                   getIntFromMap(m, "duration"),
		ChannelID:                  getStringFromMap(m, "channel_id"),
		Priority:                   getIntWithDefault(m, "priority", 10),
		MaxDurationOverflowMinutes: getIntFromMap(m, "max_duration_overflow_minutes"),
	}

	// Parse filter
	if filterMap, ok := m["filter"].(map[string]any); ok {
		block.Filter = filterFromMap(filterMap)
	}

	// Parse filler config
	if fillerMap, ok := m["filler"].(map[string]any); ok {
		block.Filler = fillerConfigFromMap(fillerMap)
	}

	// Parse series config
	if seriesList, ok := m["series"].([]any); ok {
		for _, s := range seriesList {
			if sm, ok := s.(map[string]any); ok {
				block.Series = append(block.Series, seriesConfigFromMap(sm))
			}
		}
	}

	// Parse fallback config
	if fallbackMap, ok := m["fallback"].(map[string]any); ok {
		block.Fallback = fallbackFromMap(fallbackMap)
	}

	return block
}

func filterFromMap(m map[string]any) scheduler.Filter {
	return scheduler.Filter{
		TitlePattern: getStringFromMap(m, "title_pattern"),
		Genres:       getStringSliceFromMap(m, "genres"),
		Ratings:      getStringSliceFromMap(m, "ratings"),
		YearFrom:     getIntFromMap(m, "year_from"),
		YearTo:       getIntFromMap(m, "year_to"),
		MinDuration:  getIntFromMap(m, "min_duration"),
		MaxDuration:  getIntFromMap(m, "max_duration"),
		Tags:         getStringSliceFromMap(m, "tags"),
	}
}

func fillerConfigFromMap(m map[string]any) scheduler.FillerConfig {
	return scheduler.FillerConfig{
		Enabled:       getBoolFromMap(m, "enabled"),
		FillerListID:  getStringFromMap(m, "filler_list_id"),
		MaxFillerTime: getIntFromMap(m, "max_filler_time"),
		MinGapTime:    getIntWithDefault(m, "min_gap_time", 5),
	}
}

func seriesConfigFromMap(m map[string]any) scheduler.SeriesConfig {
	return scheduler.SeriesConfig{
		ShowTitle:        getStringFromMap(m, "show_title"),
		EpisodesPerBlock: getIntWithDefault(m, "episodes_per_block", 1),
		StartSeason:      getIntWithDefault(m, "start_season", 1),
		StartEpisode:     getIntWithDefault(m, "start_episode", 1),
		OnComplete:       scheduler.CompletionAction(getStringWithDefault(m, "on_complete", "continue")),
		SkipEpisodes:     getStringSliceFromMap(m, "skip_episodes"),
		MaxRuns:          getIntFromMap(m, "max_runs"),
	}
}

func fallbackFromMap(m map[string]any) scheduler.SeriesFallback {
	fb := scheduler.SeriesFallback{
		Mode: scheduler.FallbackMode(getStringWithDefault(m, "mode", "redistribute")),
	}
	if filterMap, ok := m["filler_filter"].(map[string]any); ok {
		fb.FillerFilter = filterFromMap(filterMap)
	}
	return fb
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

func getStringWithDefault(m map[string]any, key, def string) string {
	if s := getStringFromMap(m, key); s != "" {
		return s
	}
	return def
}

func getBoolFromMap(m map[string]any, key string) bool {
	if m == nil {
		return false
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func getStringSliceFromMap(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	if v, ok := m[key].([]any); ok {
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func getIntWithDefault(m map[string]any, key string, def int) int {
	if v := getIntFromMap(m, key); v != 0 {
		return v
	}
	return def
}

// SaveSchedulerBlocks saves a slice of scheduler.Block to a YAML file.
// This is used by the TUI to persist changes to the scheduler configuration.
func SaveSchedulerBlocks(blocks []scheduler.Block, path string) error {
	if path == "" {
		path = "scheduler.yaml"
	}

	cfg := scheduler.Config{Blocks: blocks}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduler config: %w", err)
	}

	// #nosec G306 - Config file should be readable by owner
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write scheduler file %s: %w", path, err)
	}

	return nil
}

// Helper to get int from map
func getIntFromMap(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}
