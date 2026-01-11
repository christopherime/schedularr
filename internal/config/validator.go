package config

import (
	"fmt"
	"os"
	"path/filepath"

	"cuelang.org/go/cue/cuecontext"
	"github.com/geekxflood/schedularr/internal/scheduler"
)

// FindSchema returns the path to the requested schema file or error if not found.
func FindSchema(filename string) (string, error) {
	searchPaths := []string{
		filepath.Join("cmd", "schema", filename),                      // Dev (root)
		filepath.Join("schema", filename),                             // Relative
		filepath.Join("/app", "schema", filename),                     // Docker
		filepath.Join("/usr/local/share/schedularr/schema", filename), // System
	}

	for _, path := range searchPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("schema file %s not found in search paths", filename)
}

// ValidateAppConfig validates the application configuration against the CUE schema.
func ValidateAppConfig(cfg *Config) error {
	path, err := FindSchema("config.cue")
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	schemaContent, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read config schema: %w", err)
	}

	ctx := cuecontext.New()
	schema := ctx.CompileString(string(schemaContent))
	if schema.Err() != nil {
		return fmt.Errorf("failed to compile config schema: %w", schema.Err())
	}

	val := ctx.Encode(cfg)
	res := schema.Unify(val)
	if err := res.Validate(); err != nil {
		return fmt.Errorf("invalid app configuration: %w", err)
	}
	return nil
}

// ValidateSchedulerConfigStruct validates the scheduler configuration against the CUE schema.
func ValidateSchedulerConfigStruct(cfg *scheduler.Config) error {
	path, err := FindSchema("scheduler.cue")
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	schemaContent, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read scheduler schema: %w", err)
	}

	ctx := cuecontext.New()
	schema := ctx.CompileString(string(schemaContent))
	if schema.Err() != nil {
		return fmt.Errorf("failed to compile scheduler schema: %w", schema.Err())
	}

	val := ctx.Encode(cfg)
	res := schema.Unify(val)
	if err := res.Validate(); err != nil {
		return fmt.Errorf("invalid scheduler configuration: %w", err)
	}
	return nil
}