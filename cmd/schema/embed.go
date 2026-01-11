// Package schema provides embedded CUE schema definitions for configuration validation.
package schema

import (
	_ "embed"
)

// ConfigSchema contains the CUE schema for application configuration.
//
//go:embed config.cue
var ConfigSchema string

// SchedulerSchema contains the CUE schema for scheduler configuration.
//
//go:embed scheduler.cue
var SchedulerSchema string
