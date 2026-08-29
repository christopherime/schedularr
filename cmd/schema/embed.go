// Package schema provides embedded CUE schemas for Schedularr configuration.
// This package embeds the CUE schema files and provides them for use throughout
// the application. The CUE schemas are the single source of truth for configuration
// structure and default values.
package schema

import (
	_ "embed"
)

// ConfigSchema is the embedded contents of config.cue, the CUE schema for
// the application config file (structure, validation, and defaults).
//
//go:embed config.cue
var ConfigSchema string

// SchedulerSchema is the embedded contents of scheduler.cue, the CUE schema
// for the scheduler.yaml block-import file.
//
//go:embed scheduler.cue
var SchedulerSchema string
