// Package schema provides embedded CUE schemas for Schedularr configuration.
// This package embeds the CUE schema files and provides them for use throughout
// the application. The CUE schemas are the single source of truth for configuration
// structure and default values.
package schema

import (
	_ "embed"
)

//go:embed config.cue
var ConfigSchema string

//go:embed scheduler.cue
var SchedulerSchema string
