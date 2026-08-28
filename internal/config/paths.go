// Package config provides configuration loading for Schedularr.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// EnvConfigPath is the environment variable for config file path.
	EnvConfigPath = "SCHEDULARR_CONFIG"

	// EnvAPIToken is the *name* of the environment variable carrying the
	// `serve` command's bearer token -- not a credential itself, just the
	// key config.APIToken reads via os.Getenv. It always wins over the
	// api.token config key -- see APIToken.
	// #nosec G101 -- this is an env var name, not a hardcoded credential
	EnvAPIToken = "SCHEDULARR_API_TOKEN"

	// DefaultConfigDir is the default config directory name.
	DefaultConfigDir = ".schedularr"

	// DefaultDataDir is the default data directory name.
	DefaultDataDir = "schedularr"

	// DefaultConfigFile is the default config file name.
	DefaultConfigFile = "config.yaml"

	// DefaultLogsDir is the default logs directory name.
	DefaultLogsDir = "logs"
)

// Paths holds resolved paths for Schedularr directories and files.
type Paths struct {
	ConfigFile string
	ConfigDir  string
	LogsDir    string
	DataDir    string
}

// DefaultPaths returns OS-agnostic default paths based on user home directory.
// Config: $HOME/.schedularr/config.yaml
// Logs: $HOME/schedularr/logs/
func DefaultPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get user home directory: %w", err)
	}

	configDir := filepath.Join(home, DefaultConfigDir)
	dataDir := filepath.Join(home, DefaultDataDir)

	return &Paths{
		ConfigFile: filepath.Join(configDir, DefaultConfigFile),
		ConfigDir:  configDir,
		DataDir:    dataDir,
		LogsDir:    filepath.Join(dataDir, DefaultLogsDir),
	}, nil
}

// ResolvePaths determines paths using the priority: flag > env > default.
// The flagValue parameter is the --config flag value (empty if not set).
func ResolvePaths(flagValue string) (*Paths, error) {
	defaults, err := DefaultPaths()
	if err != nil {
		return nil, err
	}

	paths := defaults

	// Priority 1: Flag value
	if flagValue != "" {
		paths.ConfigFile = flagValue
		// Derive config dir from explicit config file path
		paths.ConfigDir = filepath.Dir(flagValue)
		return paths, nil
	}

	// Priority 2: Environment variable
	if envPath := os.Getenv(EnvConfigPath); envPath != "" {
		paths.ConfigFile = envPath
		paths.ConfigDir = filepath.Dir(envPath)
		return paths, nil
	}

	// Priority 3: Check legacy locations before using new defaults
	legacyPaths := getLegacyConfigPaths()
	for _, p := range legacyPaths {
		if _, err := os.Stat(p); err == nil {
			paths.ConfigFile = p
			paths.ConfigDir = filepath.Dir(p)
			return paths, nil
		}
	}

	// Priority 4: Default paths (new location)
	return paths, nil
}

// getLegacyConfigPaths returns config paths from legacy locations.
func getLegacyConfigPaths() []string {
	paths := []string{
		"config.yaml",
		".schedularr.yaml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".config", ".schedularr.yaml"),
			filepath.Join(home, ".schedularr.yaml"),
		)
	}

	return paths
}

// EnsureDirectories creates all required directories if they don't exist.
// Uses os.MkdirAll with 0750 permissions (owner: rwx, group: rx, others: none).
func EnsureDirectories(p *Paths) error {
	dirs := []string{
		p.ConfigDir,
		p.DataDir,
		p.LogsDir,
	}

	for _, dir := range dirs {
		// #nosec G301 - Directories need rx for group for potential service use
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// LogsDir returns the configured logs directory from the app config,
// or the default path if not configured.
func LogsDir(cfg *Config) string {
	if cfg != nil {
		if dir := cfg.GetString("logs_dir"); dir != "" {
			return dir
		}
	}

	defaults, err := DefaultPaths()
	if err != nil {
		return filepath.Join(".", DefaultLogsDir)
	}
	return defaults.LogsDir
}

// FindConfigFile searches for a config file in standard locations.
// Returns the path to the first existing config file, or empty string if none found.
func FindConfigFile(flagValue string) string {
	// Priority 1: Flag value
	if flagValue != "" {
		if _, err := os.Stat(flagValue); err == nil {
			return flagValue
		}
		return ""
	}

	// Priority 2: Environment variable
	if envPath := os.Getenv(EnvConfigPath); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
		return ""
	}

	// Priority 3: Legacy locations
	legacyPaths := getLegacyConfigPaths()
	for _, p := range legacyPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	// Priority 4: New default location
	defaults, err := DefaultPaths()
	if err != nil {
		return ""
	}

	if _, err := os.Stat(defaults.ConfigFile); err == nil {
		return defaults.ConfigFile
	}

	return ""
}
