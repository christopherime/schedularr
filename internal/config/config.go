package config

import (
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/geekxflood/schedularr/internal/tunarr"
)

// Config holds the application configuration
type Config struct {
	Tunarr    tunarr.Config    `mapstructure:"tunarr" yaml:"tunarr"`
	Log       LogConfig        `mapstructure:"log" yaml:"log"`
	Scheduler scheduler.Config `mapstructure:"scheduler" yaml:"scheduler"`
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
	}
}