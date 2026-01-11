package config

import (
	"fmt"

	"cuelang.org/go/cue/cuecontext"
	"github.com/geekxflood/schedularr/cmd/schema"
	"github.com/geekxflood/schedularr/internal/scheduler"
)

// ValidateAppConfig validates the application configuration against the CUE schema.
func ValidateAppConfig(cfg *Config) error {
	ctx := cuecontext.New()
	cueSchema := ctx.CompileString(schema.ConfigSchema)
	if cueSchema.Err() != nil {
		return fmt.Errorf("failed to compile config schema: %w", cueSchema.Err())
	}

	val := ctx.Encode(cfg)
	res := cueSchema.Unify(val)
	if err := res.Validate(); err != nil {
		return fmt.Errorf("invalid app configuration: %w", err)
	}
	return nil
}

// ValidateSchedulerConfigStruct validates the scheduler configuration against the CUE schema.
func ValidateSchedulerConfigStruct(cfg *scheduler.Config) error {
	ctx := cuecontext.New()
	cueSchema := ctx.CompileString(schema.SchedulerSchema)
	if cueSchema.Err() != nil {
		return fmt.Errorf("failed to compile scheduler schema: %w", cueSchema.Err())
	}

	val := ctx.Encode(cfg)
	res := cueSchema.Unify(val)
	if err := res.Validate(); err != nil {
		return fmt.Errorf("invalid scheduler configuration: %w", err)
	}
	return nil
}