package cueconfig

import (
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

	// Validate the generated config
	if err := v.ValidateConfig(data, "json"); err != nil {
		t.Errorf("Generated config is invalid: %v", err)
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

	// Validate the generated scheduler
	if err := v.ValidateScheduler(data, "json"); err != nil {
		t.Errorf("Generated scheduler is invalid: %v", err)
	}

	// Check that it contains expected fields
	dataStr := string(data)
	if !strings.Contains(dataStr, "blocks") {
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

func TestValidateConfigStruct(t *testing.T) {
	v := NewValidator()

	// Test with a valid struct
	type TestConfig struct {
		Tunarr struct {
			URL string `yaml:"url"`
		} `yaml:"tunarr"`
		Log struct {
			Level  string `yaml:"level"`
			Format string `yaml:"format"`
		} `yaml:"log"`
	}

	validConfig := TestConfig{}
	validConfig.Tunarr.URL = "http://localhost:8000"
	validConfig.Log.Level = "info"
	validConfig.Log.Format = "text"

	err := v.ValidateConfigStruct(validConfig)
	if err != nil {
		t.Errorf("ValidateConfigStruct failed for valid config: %v", err)
	}

	// Test with invalid struct (invalid log level)
	invalidConfig := TestConfig{}
	invalidConfig.Tunarr.URL = "http://localhost:8000"
	invalidConfig.Log.Level = "invalid"
	invalidConfig.Log.Format = "text"

	err = v.ValidateConfigStruct(invalidConfig)
	if err == nil {
		t.Error("Expected validation error for invalid log level, got nil")
	}
}

func TestValidateSchedulerStruct(t *testing.T) {
	v := NewValidator()

	// Test with a valid struct
	type TestScheduler struct {
		Blocks []map[string]interface{} `yaml:"blocks"`
	}

	validScheduler := TestScheduler{
		Blocks: []map[string]interface{}{
			{
				"name":       "Test Block",
				"type":       "filter",
				"cron":       "0 9 * * *",
				"duration":   120,
				"channel_id": "channel-1",
				"priority":   10,
			},
		},
	}

	err := v.ValidateSchedulerStruct(validScheduler)
	if err != nil {
		t.Errorf("ValidateSchedulerStruct failed for valid scheduler: %v", err)
	}

	// Test with invalid struct (negative duration)
	invalidScheduler := TestScheduler{
		Blocks: []map[string]interface{}{
			{
				"name":       "Test Block",
				"type":       "filter",
				"cron":       "0 9 * * *",
				"duration":   -10,
				"channel_id": "channel-1",
				"priority":   10,
			},
		},
	}

	err = v.ValidateSchedulerStruct(invalidScheduler)
	if err == nil {
		t.Error("Expected validation error for negative duration, got nil")
	}
}
