// Package cueconfig provides CUE schema-based configuration validation and generation.
package cueconfig

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
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

// ValidateConfigStruct validates a Config struct by marshaling it to YAML first
func (v *SchemaValidator) ValidateConfigStruct(cfg interface{}) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	return v.ValidateConfig(data, "yaml")
}

// ValidateSchedulerStruct validates a Scheduler config struct by marshaling it to YAML first
func (v *SchemaValidator) ValidateSchedulerStruct(cfg interface{}) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal scheduler config: %w", err)
	}
	return v.ValidateScheduler(data, "yaml")
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
	unified := schemaValue.LookupPath(cue.ParsePath("#Config")).Unify(dataValue)
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
	unified := schemaValue.LookupPath(cue.ParsePath("#SchedulerFile")).Unify(dataValue)
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
	defaultConfig := schemaValue.LookupPath(cue.ParsePath("config"))
	if defaultConfig.Err() != nil {
		return nil, fmt.Errorf("failed to get default config: %w", defaultConfig.Err())
	}

	// Convert to Go value
	var configData interface{}
	if err := defaultConfig.Decode(&configData); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	// Convert to requested format
	switch format {
	case "yaml", "yml":
		return yaml.Marshal(configData)
	case "json":
		return json.MarshalIndent(configData, "", "  ")
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
	defaultScheduler := schemaValue.LookupPath(cue.ParsePath("scheduler"))
	if defaultScheduler.Err() != nil {
		return nil, fmt.Errorf("failed to get default scheduler: %w", defaultScheduler.Err())
	}

	// Convert to Go value
	var schedulerData interface{}
	if err := defaultScheduler.Decode(&schedulerData); err != nil {
		return nil, fmt.Errorf("failed to decode scheduler: %w", err)
	}

	// Convert to requested format
	switch format {
	case "yaml", "yml":
		return yaml.Marshal(schedulerData)
	case "json":
		return json.MarshalIndent(schedulerData, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}
