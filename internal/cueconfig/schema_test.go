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
