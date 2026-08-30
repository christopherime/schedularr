package cueconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if v == nil {
		t.Fatal("NewValidator() returned nil")
	}
	if v.ctx == nil {
		t.Fatal("NewValidator() returned validator with nil context")
	}
}

func TestValidateConfig_ValidYAML(t *testing.T) {
	v := NewValidator()

	validConfig := `
tunarr:
  url: "http://localhost:8000"
log:
  level: "info"
  format: "text"
`

	err := v.ValidateConfig([]byte(validConfig), "yaml")
	if err != nil {
		t.Errorf("ValidateConfig failed for valid config: %v", err)
	}
}

func TestValidateConfig_ValidJSON(t *testing.T) {
	v := NewValidator()

	validConfig := `{
  "tunarr": {
    "url": "http://localhost:8000"
  },
  "log": {
    "level": "info",
    "format": "text"
  }
}`

	err := v.ValidateConfig([]byte(validConfig), "json")
	if err != nil {
		t.Errorf("ValidateConfig failed for valid config: %v", err)
	}
}

func TestValidateConfig_InvalidLogLevel(t *testing.T) {
	v := NewValidator()

	invalidConfig := `
tunarr:
  url: "http://localhost:8000"
log:
  level: "invalid"
  format: "text"
`

	err := v.ValidateConfig([]byte(invalidConfig), "yaml")
	if err == nil {
		t.Error("Expected validation error for invalid log level, got nil")
	}
}

func TestValidateConfig_InvalidYAML(t *testing.T) {
	v := NewValidator()

	invalidConfig := `
tunarr:
  url: "http://localhost:8000"
log:
  level: "info"
  format: "text"
  invalid_field: true
`

	// This should still pass because CUE allows additional fields by default
	// Just testing that it doesn't crash
	_ = v.ValidateConfig([]byte(invalidConfig), "yaml")
}

func TestValidateConfig_UnsupportedFormat(t *testing.T) {
	v := NewValidator()

	err := v.ValidateConfig([]byte("test"), "xml")
	if err == nil {
		t.Error("Expected error for unsupported format, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("Expected 'unsupported format' error, got: %v", err)
	}
}

func TestValidateScheduler_ValidYAML(t *testing.T) {
	v := NewValidator()

	validScheduler := `
blocks:
  - name: "Test Block"
    type: "filter"
    cron: "0 9 * * *"
    duration: 120
    channel_id: "channel-1"
    priority: 10
    filter:
      genres: ["Comedy"]
`

	err := v.ValidateScheduler([]byte(validScheduler), "yaml")
	if err != nil {
		t.Errorf("ValidateScheduler failed for valid scheduler: %v", err)
	}
}

func TestValidateScheduler_MissingRequired(t *testing.T) {
	v := NewValidator()

	invalidScheduler := `
blocks:
  - name: "Test Block"
    type: "filter"
    cron: "0 9 * * *"
`

	err := v.ValidateScheduler([]byte(invalidScheduler), "yaml")
	if err == nil {
		t.Error("Expected validation error for missing required fields, got nil")
	}
}

func TestGenerateConfig_YAML(t *testing.T) {
	v := NewValidator()

	data, err := v.GenerateConfig("yaml")
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("GenerateConfig returned empty data")
	}

	// Validate the generated config
	if err := v.ValidateConfig(data, "yaml"); err != nil {
		t.Errorf("Generated config is invalid: %v", err)
	}
}

func TestGenerateConfig_JSON(t *testing.T) {
	v := NewValidator()

	data, err := v.GenerateConfig("json")
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("GenerateConfig returned empty data")
	}

	// Verify it's valid JSON by parsing it
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("Generated JSON is invalid: %v", err)
	}

	// Check for expected top-level keys
	if _, ok := parsed["tunarr"]; !ok {
		t.Error("Generated config missing 'tunarr' key")
	}
	if _, ok := parsed["log"]; !ok {
		t.Error("Generated config missing 'log' key")
	}
}

func TestGenerateScheduler_YAML(t *testing.T) {
	v := NewValidator()

	data, err := v.GenerateScheduler("yaml")
	if err != nil {
		t.Fatalf("GenerateScheduler failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("GenerateScheduler returned empty data")
	}

	// Validate the generated scheduler
	if err := v.ValidateScheduler(data, "yaml"); err != nil {
		t.Errorf("Generated scheduler is invalid: %v", err)
	}

	// Check that it contains expected fields
	dataStr := string(data)
	if !strings.Contains(dataStr, "blocks:") {
		t.Error("Generated scheduler missing 'blocks' field")
	}
	if !strings.Contains(dataStr, "name:") {
		t.Error("Generated scheduler missing 'name' field")
	}
	if !strings.Contains(dataStr, "type:") {
		t.Error("Generated scheduler missing 'type' field")
	}
}

func TestGenerateScheduler_JSON(t *testing.T) {
	v := NewValidator()

	data, err := v.GenerateScheduler("json")
	if err != nil {
		t.Fatalf("GenerateScheduler failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("GenerateScheduler returned empty data")
	}

	// Verify it's valid JSON by parsing it
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Errorf("Generated JSON is invalid: %v", err)
	}

	// Check for expected fields
	if _, ok := parsed["blocks"]; !ok {
		t.Error("Generated scheduler missing 'blocks' field")
	}
}

func TestGenerateScheduler_UnsupportedFormat(t *testing.T) {
	v := NewValidator()

	_, err := v.GenerateScheduler("xml")
	if err == nil {
		t.Error("Expected error for unsupported format, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("Expected 'unsupported format' error, got: %v", err)
	}
}

func TestValidateScheduler_InvalidBlockType(t *testing.T) {
	v := NewValidator()

	invalidScheduler := `
blocks:
  - name: "Test Block"
    type: "invalid_type"
    cron: "0 9 * * *"
    duration: 120
    channel_id: "channel-1"
    priority: 10
`

	err := v.ValidateScheduler([]byte(invalidScheduler), "yaml")
	if err == nil {
		t.Error("Expected validation error for invalid block type, got nil")
	}
}

func TestValidateScheduler_NegativeDuration(t *testing.T) {
	v := NewValidator()

	invalidScheduler := `
blocks:
  - name: "Test Block"
    type: "filter"
    cron: "0 9 * * *"
    duration: -10
    channel_id: "channel-1"
    priority: 10
`

	err := v.ValidateScheduler([]byte(invalidScheduler), "yaml")
	if err == nil {
		t.Error("Expected validation error for negative duration, got nil")
	}
}

func TestValidateConfig_InvalidFormat(t *testing.T) {
	v := NewValidator()

	invalidConfig := `
tunarr:
  url: "http://localhost:8000"
log:
  level: "info"
  format: "invalid_format"
`

	err := v.ValidateConfig([]byte(invalidConfig), "yaml")
	if err == nil {
		t.Error("Expected validation error for invalid log format, got nil")
	}
}

func TestGenerateConfig_UnsupportedFormat(t *testing.T) {
	v := NewValidator()

	_, err := v.GenerateConfig("xml")
	if err == nil {
		t.Error("Expected error for unsupported format, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("Expected 'unsupported format' error, got: %v", err)
	}
}

func TestValidateConfig_MalformedYAML(t *testing.T) {
	v := NewValidator()

	malformedYAML := `
tunarr:
  url: "http://localhost:8000"
log:
  level: info
  format: [this is invalid yaml
`

	err := v.ValidateConfig([]byte(malformedYAML), "yaml")
	if err == nil {
		t.Error("Expected error for malformed YAML, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse YAML") {
		t.Errorf("Expected 'failed to parse YAML' error, got: %v", err)
	}
}

func TestValidateConfig_MalformedJSON(t *testing.T) {
	v := NewValidator()

	malformedJSON := `{
  "tunarr": {
    "url": "http://localhost:8000"
  },
  "log": {
    "level": "info"
  ` // Missing closing braces

	err := v.ValidateConfig([]byte(malformedJSON), "json")
	if err == nil {
		t.Error("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON") {
		t.Errorf("Expected 'failed to parse JSON' error, got: %v", err)
	}
}

func TestValidateScheduler_MalformedYAML(t *testing.T) {
	v := NewValidator()

	malformedYAML := `
blocks:
  - name: "Test Block"
    type: filter
    cron: [this is invalid yaml
`

	err := v.ValidateScheduler([]byte(malformedYAML), "yaml")
	if err == nil {
		t.Error("Expected error for malformed YAML, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse YAML") {
		t.Errorf("Expected 'failed to parse YAML' error, got: %v", err)
	}
}

func TestValidateScheduler_MalformedJSON(t *testing.T) {
	v := NewValidator()

	malformedJSON := `{
  "blocks": [
    {
      "name": "Test Block",
      "type": "filter"
  ` // Missing closing braces

	err := v.ValidateScheduler([]byte(malformedJSON), "json")
	if err == nil {
		t.Error("Expected error for malformed JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON") {
		t.Errorf("Expected 'failed to parse JSON' error, got: %v", err)
	}
}

func TestValidateScheduler_UnsupportedFormat(t *testing.T) {
	v := NewValidator()

	err := v.ValidateScheduler([]byte("test"), "xml")
	if err == nil {
		t.Error("Expected error for unsupported format, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("Expected 'unsupported format' error, got: %v", err)
	}
}

// TestLoadConfigWithEnvInterpolation_UnsetVarBecomesEmptyString pins the
// post-parse interpolation contract: an unset ${VAR} placeholder -- quoted
// or unquoted -- yields an empty STRING in the loaded config, never a YAML
// null. The pre-fix textual os.ExpandEnv left an empty unquoted token that
// parsed as null and failed #Config validation with a type error.
func TestLoadConfigWithEnvInterpolation_UnsetVarBecomesEmptyString(t *testing.T) {
	t.Setenv("SCHEDULARR_TEST_SET_VAR", "http://tunarr:8000")
	// Deliberately never set SCHEDULARR_TEST_UNSET_VAR.

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "tunarr:\n" +
		"  url: ${SCHEDULARR_TEST_SET_VAR}\n" +
		"  api_key: ${SCHEDULARR_TEST_UNSET_VAR}\n" + // unquoted, unset
		"database: \"${SCHEDULARR_TEST_UNSET_VAR}\"\n" // quoted, unset
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewValidator().LoadConfigWithEnvInterpolation(path)
	if err != nil {
		t.Fatalf("LoadConfigWithEnvInterpolation() error: %v", err)
	}

	tunarr, ok := cfg.data["tunarr"].(map[string]any)
	if !ok {
		t.Fatalf("tunarr section missing or wrong type: %#v", cfg.data["tunarr"])
	}
	if got := tunarr["url"]; got != "http://tunarr:8000" {
		t.Errorf("set variable not expanded: url = %#v", got)
	}
	if got := tunarr["api_key"]; got != "" {
		t.Errorf("unset unquoted variable: api_key = %#v, want \"\"", got)
	}
	if got := cfg.data["database"]; got != "" {
		t.Errorf("unset quoted variable: database = %#v, want \"\"", got)
	}
}

// TestLoadConfigWithEnvInterpolation_ValueNeverParsedAsYAML pins the other
// benefit of post-parse interpolation: a variable's VALUE is substituted
// into an already-parsed string and can never inject structure into the
// document, no matter what YAML-looking text it contains.
func TestLoadConfigWithEnvInterpolation_ValueNeverParsedAsYAML(t *testing.T) {
	injected := "x\"\nlog:\n  level: debug"
	t.Setenv("SCHEDULARR_TEST_INJECT_VAR", injected)

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := "tunarr:\n  url: ${SCHEDULARR_TEST_INJECT_VAR}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := NewValidator().LoadConfigWithEnvInterpolation(path)
	if err != nil {
		t.Fatalf("LoadConfigWithEnvInterpolation() error: %v", err)
	}

	tunarr := cfg.data["tunarr"].(map[string]any)
	if got := tunarr["url"]; got != injected {
		t.Errorf("value was not kept as a literal string: url = %#v", got)
	}
	// The injected "log:" text must not have become a real log section
	// override: the schema default ("info") applies, not "debug".
	log, ok := cfg.data["log"].(map[string]any)
	if !ok {
		t.Fatalf("log section missing: %#v", cfg.data["log"])
	}
	if got := log["level"]; got != "info" {
		t.Errorf("injected text changed document structure: log.level = %#v, want schema default \"info\"", got)
	}
}
