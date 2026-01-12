// Package cueconfig provides CUE schema-based configuration validation and generation.
package cueconfig

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueyaml "cuelang.org/go/encoding/yaml"
	"gopkg.in/yaml.v3"
)

//go:embed schema/config.cue
var configSchema string

//go:embed schema/scheduler.cue
var schedulerSchema string

// SchemaValidator provides CUE schema validation
type SchemaValidator struct {
	ctx *cue.Context
}

// NewValidator creates a new schema validator
func NewValidator() *SchemaValidator {
	return &SchemaValidator{
		ctx: cuecontext.New(),
	}
}

// ValidateConfig validates a configuration file against the config schema
func (v *SchemaValidator) ValidateConfig(data []byte, format string) error {
	// Compile the schema
	schemaValue := v.ctx.CompileString(configSchema)
	if schemaValue.Err() != nil {
		return fmt.Errorf("failed to compile config schema: %w", schemaValue.Err())
	}

	// Parse the data based on format
	var dataValue cue.Value

	switch format {
	case "yaml", "yml":
		// Parse YAML to generic map
		var yamlData map[string]interface{}
		if err := yaml.Unmarshal(data, &yamlData); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}
		// Encode the map as CUE value
		dataValue = v.ctx.Encode(yamlData)
	case "json":
		dataValue = v.ctx.CompileBytes(data)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	if dataValue.Err() != nil {
		return fmt.Errorf("failed to parse data: %w", dataValue.Err())
	}

	// Unify with schema
	unified := schemaValue.LookupPath(cue.ParsePath("schema.#Config")).Unify(dataValue)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// ValidateScheduler validates a scheduler configuration file
func (v *SchemaValidator) ValidateScheduler(data []byte, format string) error {
	// Compile the schema
	schemaValue := v.ctx.CompileString(schedulerSchema)
	if schemaValue.Err() != nil {
		return fmt.Errorf("failed to compile scheduler schema: %w", schemaValue.Err())
	}

	// Parse the data based on format
	var dataValue cue.Value

	switch format {
	case "yaml", "yml":
		// Parse YAML to generic map
		var yamlData map[string]interface{}
		if err := yaml.Unmarshal(data, &yamlData); err != nil {
			return fmt.Errorf("failed to parse YAML: %w", err)
		}
		// Encode the map as CUE value
		dataValue = v.ctx.Encode(yamlData)
	case "json":
		dataValue = v.ctx.CompileBytes(data)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}

	if dataValue.Err() != nil {
		return fmt.Errorf("failed to parse data: %w", dataValue.Err())
	}

	// Unify with schema
	unified := schemaValue.LookupPath(cue.ParsePath("schema.#SchedulerFile")).Unify(dataValue)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// GenerateConfig generates a configuration file from the schema with defaults
func (v *SchemaValidator) GenerateConfig(format string) ([]byte, error) {
	// Compile the schema
	schemaValue := v.ctx.CompileString(configSchema)
	if schemaValue.Err() != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", schemaValue.Err())
	}

	// Get the default config instance
	defaultConfig := schemaValue.LookupPath(cue.ParsePath("schema.config"))
	if defaultConfig.Err() != nil {
		return nil, fmt.Errorf("failed to get default config: %w", defaultConfig.Err())
	}

	// Convert to requested format
	switch format {
	case "yaml", "yml":
		return yaml.Encode(defaultConfig)
	case "json":
		return json.MarshalIndent(defaultConfig, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateScheduler generates a scheduler configuration file from the schema with defaults
func (v *SchemaValidator) GenerateScheduler(format string) ([]byte, error) {
	// Compile the schema
	schemaValue := v.ctx.CompileString(schedulerSchema)
	if schemaValue.Err() != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", schemaValue.Err())
	}

	// Get the default scheduler instance
	defaultScheduler := schemaValue.LookupPath(cue.ParsePath("schema.scheduler"))
	if defaultScheduler.Err() != nil {
		return nil, fmt.Errorf("failed to get default scheduler: %w", defaultScheduler.Err())
	}

	// Convert to requested format
	switch format {
	case "yaml", "yml":
		return yaml.Encode(defaultScheduler)
	case "json":
		return json.MarshalIndent(defaultScheduler, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}
