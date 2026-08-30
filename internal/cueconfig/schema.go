// Package cueconfig provides CUE schema-based configuration validation, generation, and loading.
// The CUE schemas in cmd/schema/ are the single source of truth for configuration structure
// and default values. This package provides methods to work with configuration data without
// requiring explicit Go type definitions.
package cueconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/christopherime/schedularr/cmd/schema"
	"gopkg.in/yaml.v3"
)

// Config represents the application configuration loaded from CUE.
// It wraps a map[string]any and provides typed accessor methods.
type Config struct {
	data map[string]any
}

// SchemaValidator provides CUE schema validation and config loading
type SchemaValidator struct {
	ctx *cue.Context
}

// NewValidator creates a new schema validator
func NewValidator() *SchemaValidator {
	return &SchemaValidator{
		ctx: cuecontext.New(),
	}
}

// parseInput parses YAML/JSON bytes into the generic map every validation
// and load path in this package feeds to CUE. Shared by LoadConfig,
// ValidateConfig, ValidateScheduler, and LoadConfigWithEnvInterpolation.
func parseInput(data []byte, format string) (map[string]any, error) {
	var inputData map[string]any
	switch format {
	case "yaml", "yml":
		if err := yaml.Unmarshal(data, &inputData); err != nil {
			return nil, fmt.Errorf("failed to parse YAML: %w", err)
		}
	case "json":
		if err := json.Unmarshal(data, &inputData); err != nil {
			return nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
	return inputData, nil
}

// LoadConfig loads configuration from YAML/JSON data, validates it against the schema,
// and returns a Config with defaults filled in from the CUE schema.
func (v *SchemaValidator) LoadConfig(data []byte, format string) (*Config, error) {
	inputData, err := parseInput(data, format)
	if err != nil {
		return nil, err
	}
	return v.loadParsed(inputData)
}

// loadParsed validates an already-parsed config map against #Config and
// returns a Config with the schema's defaults filled in -- LoadConfig's
// back half, split out so LoadConfigWithEnvInterpolation can interpolate
// environment variables on the parsed map (between parse and validation)
// rather than on the raw text.
func (v *SchemaValidator) loadParsed(inputData map[string]any) (*Config, error) {
	// Compile the schema
	schemaValue := v.ctx.CompileString(schema.ConfigSchema)
	if schemaValue.Err() != nil {
		return nil, fmt.Errorf("failed to compile config schema: %w", schemaValue.Err())
	}

	// Get the #Config definition
	configDef := schemaValue.LookupPath(cue.ParsePath("#Config"))
	if configDef.Err() != nil {
		return nil, fmt.Errorf("failed to get #Config definition: %w", configDef.Err())
	}

	// Encode input as CUE value
	inputValue := v.ctx.Encode(inputData)
	if inputValue.Err() != nil {
		return nil, fmt.Errorf("failed to encode input data: %w", inputValue.Err())
	}

	// Unify with schema to get defaults filled in
	unified := configDef.Unify(inputValue)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Decode the unified value back to a map
	var configData map[string]any
	if err := unified.Decode(&configData); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	return &Config{data: configData}, nil
}

// LoadConfigWithEnvInterpolation loads config from file, expanding
// ${VAR}/$VAR environment-variable placeholders inside its string values.
//
// Interpolation happens AFTER parsing, on the parsed map's string values
// only -- not textually on the raw bytes, which is what an earlier
// os.ExpandEnv-based version did. Post-parse expansion closes two textual
// footguns at once: an unset variable now yields an empty STRING (the raw
// expansion left an empty unquoted YAML token, which parses as null and
// then fails schema validation with a confusing type error -- quoted or
// unquoted, the placeholder's spot stays a string now), and a variable's
// VALUE can never be re-parsed as YAML/JSON syntax, since the document's
// structure is fixed before any value is substituted.
func (v *SchemaValidator) LoadConfigWithEnvInterpolation(path string) (*Config, error) {
	// Read file
	// #nosec G304 - path is the operator's own --config/SCHEDULARR_CONFIG value
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Determine format from extension
	format := "yaml"
	if strings.HasSuffix(path, ".json") {
		format = "json"
	}

	inputData, err := parseInput(data, format)
	if err != nil {
		return nil, err
	}

	expanded, _ := expandEnvInStrings(inputData).(map[string]any)
	return v.loadParsed(expanded)
}

// expandEnvInStrings returns value with every string it contains --
// directly, in nested maps, or in slices -- passed through
// os.Expand(s, os.Getenv), replacing ${VAR}/$VAR references with the
// variable's value (empty string when unset). Non-string leaves are
// returned unchanged; the result of expanding a string is always a string.
func expandEnvInStrings(value any) any {
	switch t := value.(type) {
	case string:
		return os.Expand(t, os.Getenv)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, v := range t {
			out[k] = expandEnvInStrings(v)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, v := range t {
			out[i] = expandEnvInStrings(v)
		}
		return out
	default:
		return value
	}
}

// ValidateConfig validates configuration data against the schema without loading
func (v *SchemaValidator) ValidateConfig(data []byte, format string) error {
	schemaValue := v.ctx.CompileString(schema.ConfigSchema)
	if schemaValue.Err() != nil {
		return fmt.Errorf("failed to compile config schema: %w", schemaValue.Err())
	}

	inputData, err := parseInput(data, format)
	if err != nil {
		return err
	}

	inputValue := v.ctx.Encode(inputData)
	if inputValue.Err() != nil {
		return fmt.Errorf("failed to encode input data: %w", inputValue.Err())
	}

	unified := schemaValue.LookupPath(cue.ParsePath("#Config")).Unify(inputValue)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// ValidateScheduler validates scheduler configuration data
func (v *SchemaValidator) ValidateScheduler(data []byte, format string) error {
	combinedSchema := combineSchemas(schema.ConfigSchema, schema.SchedulerSchema)
	schemaValue := v.ctx.CompileString(combinedSchema)
	if schemaValue.Err() != nil {
		return fmt.Errorf("failed to compile scheduler schema: %w", schemaValue.Err())
	}

	inputData, err := parseInput(data, format)
	if err != nil {
		return err
	}

	inputValue := v.ctx.Encode(inputData)
	if inputValue.Err() != nil {
		return fmt.Errorf("failed to encode input data: %w", inputValue.Err())
	}

	unified := schemaValue.LookupPath(cue.ParsePath("#SchedulerFile")).Unify(inputValue)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}

// GenerateConfig generates a default configuration from the schema
func (v *SchemaValidator) GenerateConfig(format string) ([]byte, error) {
	schemaValue := v.ctx.CompileString(schema.ConfigSchema)
	if schemaValue.Err() != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", schemaValue.Err())
	}

	defaultConfig := schemaValue.LookupPath(cue.ParsePath("config"))
	if defaultConfig.Err() != nil {
		return nil, fmt.Errorf("failed to get default config: %w", defaultConfig.Err())
	}

	var configData any
	if err := defaultConfig.Decode(&configData); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	switch format {
	case "yaml", "yml":
		return yaml.Marshal(configData)
	case "json":
		return json.MarshalIndent(configData, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateConfigWithOverrides generates configuration with optional overrides
func (v *SchemaValidator) GenerateConfigWithOverrides(format string, overrides map[string]any) ([]byte, error) {
	schemaValue := v.ctx.CompileString(schema.ConfigSchema)
	if schemaValue.Err() != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", schemaValue.Err())
	}

	defaultConfig := schemaValue.LookupPath(cue.ParsePath("config"))
	if defaultConfig.Err() != nil {
		return nil, fmt.Errorf("failed to get default config: %w", defaultConfig.Err())
	}

	var configData map[string]any
	if err := defaultConfig.Decode(&configData); err != nil {
		return nil, fmt.Errorf("failed to decode config: %w", err)
	}

	mergeMap(configData, overrides)

	switch format {
	case "yaml", "yml":
		return yaml.Marshal(configData)
	case "json":
		return json.MarshalIndent(configData, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// GenerateScheduler generates a default scheduler configuration
func (v *SchemaValidator) GenerateScheduler(format string) ([]byte, error) {
	combinedSchema := combineSchemas(schema.ConfigSchema, schema.SchedulerSchema)
	schemaValue := v.ctx.CompileString(combinedSchema)
	if schemaValue.Err() != nil {
		return nil, fmt.Errorf("failed to compile schema: %w", schemaValue.Err())
	}

	defaultScheduler := schemaValue.LookupPath(cue.ParsePath("scheduler"))
	if defaultScheduler.Err() != nil {
		return nil, fmt.Errorf("failed to get default scheduler: %w", defaultScheduler.Err())
	}

	var schedulerData any
	if err := defaultScheduler.Decode(&schedulerData); err != nil {
		return nil, fmt.Errorf("failed to decode scheduler: %w", err)
	}

	switch format {
	case "yaml", "yml":
		return yaml.Marshal(schedulerData)
	case "json":
		return json.MarshalIndent(schedulerData, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// combineSchemas combines multiple CUE schemas, stripping duplicate package declarations
func combineSchemas(schemas ...string) string {
	if len(schemas) == 0 {
		return ""
	}
	if len(schemas) == 1 {
		return schemas[0]
	}

	var b strings.Builder
	b.WriteString(schemas[0])
	for i := 1; i < len(schemas); i++ {
		// Strip "package schema" line from subsequent schemas
		s := schemas[i]
		lines := strings.Split(s, "\n")
		filteredLines := make([]string, 0, len(lines))
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "package ") {
				continue
			}
			filteredLines = append(filteredLines, line)
		}
		b.WriteByte('\n')
		b.WriteString(strings.Join(filteredLines, "\n"))
	}
	return b.String()
}

// mergeMap recursively merges src into dst, overwriting existing values.
func mergeMap(dst, src map[string]any) {
	for key, srcVal := range src {
		if srcMap, ok := srcVal.(map[string]any); ok {
			if dstVal, exists := dst[key]; exists {
				if dstMap, ok := dstVal.(map[string]any); ok {
					mergeMap(dstMap, srcMap)
					continue
				}
			}
			newMap := make(map[string]any)
			mergeMap(newMap, srcMap)
			dst[key] = newMap
		} else {
			dst[key] = srcVal
		}
	}
}

// ============================================================================
// Config accessor methods - provide typed access to configuration values
// ============================================================================

// Get returns a value at the given path (dot-separated)
func (c *Config) Get(path string) any {
	return getPath(c.data, strings.Split(path, "."))
}

// GetString returns a string value at the given path
func (c *Config) GetString(path string) string {
	if v := c.Get(path); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt returns an int value at the given path
func (c *Config) GetInt(path string) int {
	if v := c.Get(path); v != nil {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}

// GetBool returns a bool value at the given path
func (c *Config) GetBool(path string) bool {
	if v := c.Get(path); v != nil {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// GetDuration returns a time.Duration value at the given path
func (c *Config) GetDuration(path string) time.Duration {
	s := c.GetString(path)
	if s == "" {
		return 0
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

// GetMap returns a map value at the given path
func (c *Config) GetMap(path string) map[string]any {
	if v := c.Get(path); v != nil {
		if m, ok := v.(map[string]any); ok {
			return m
		}
	}
	return nil
}

// ToYAML serializes the config to YAML
func (c *Config) ToYAML() ([]byte, error) {
	return yaml.Marshal(c.data)
}

// ============================================================================
// Helper functions
// ============================================================================

// getPath traverses a nested map using path segments
func getPath(data map[string]any, path []string) any {
	if len(path) == 0 {
		return data
	}

	current := path[0]
	remaining := path[1:]

	val, exists := data[current]
	if !exists {
		return nil
	}

	if len(remaining) == 0 {
		return val
	}

	if nested, ok := val.(map[string]any); ok {
		return getPath(nested, remaining)
	}

	return nil
}
