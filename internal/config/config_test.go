package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/christopherime/schedularr/internal/cueconfig"
)

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `tunarr:
  url: "http://test-tunarr:8000"
  api_key: "test-key"
log:
  level: "debug"
  format: "json"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}

	// Test GetString accessor
	if url := cfg.GetString("tunarr.url"); url != "http://test-tunarr:8000" {
		t.Errorf("Expected tunarr.url 'http://test-tunarr:8000', got '%s'", url)
	}

	if level := cfg.GetString("log.level"); level != "debug" {
		t.Errorf("Expected log.level 'debug', got '%s'", level)
	}
}

func TestTunarrConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `tunarr:
  url: "http://my-tunarr:8000"
  api_key: "my-api-key"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	tunarrCfg := TunarrConfig(cfg)

	if tunarrCfg.URL != "http://my-tunarr:8000" {
		t.Errorf("Expected Tunarr URL 'http://my-tunarr:8000', got '%s'", tunarrCfg.URL)
	}

	if tunarrCfg.APIKey != "my-api-key" {
		t.Errorf("Expected Tunarr APIKey 'my-api-key', got '%s'", tunarrCfg.APIKey)
	}
}

func TestLogLevel(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `log:
  level: "warn"
  format: "text"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if level := LogLevel(cfg); level != "warn" {
		t.Errorf("Expected log level 'warn', got '%s'", level)
	}

	if format := LogFormat(cfg); format != "text" {
		t.Errorf("Expected log format 'text', got '%s'", format)
	}
}

func TestDatabasePath(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `database: "/var/lib/schedularr/data.db"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if dbPath := DatabasePath(cfg); dbPath != "/var/lib/schedularr/data.db" {
		t.Errorf("Expected database path '/var/lib/schedularr/data.db', got '%s'", dbPath)
	}
}

func TestSchedulerFilePath(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `scheduler_file: "/custom/path/scheduler.yaml"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got := SchedulerFilePath(cfg); got != "/custom/path/scheduler.yaml" {
		t.Errorf("Expected scheduler_file '/custom/path/scheduler.yaml', got '%s'", got)
	}
}

// TestSchedulerFilePath_DefaultsWhenOmitted verifies the CUE schema default
// for scheduler_file (cmd/schema/config.cue: `scheduler_file: string |
// *"scheduler.yaml"`) actually reaches SchedulerFilePath when a config file
// doesn't set the key. SchedulerFilePath dropped the old $HOME-search
// fallback that FindSchedulerConfig used to have when scheduler_file was
// unset; this test is the guarantee that the CUE default (not an empty
// string) is what blockio.Bootstrap gets instead.
func TestSchedulerFilePath_DefaultsWhenOmitted(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// No scheduler_file key at all.
	content := `tunarr:
  url: "http://localhost:8000"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got := SchedulerFilePath(cfg); got != "scheduler.yaml" {
		t.Errorf("Expected CUE-schema default 'scheduler.yaml' when scheduler_file is omitted, got %q", got)
	}
}

func TestGenerateDefaultConfig(t *testing.T) {
	validator := cueconfig.NewValidator()
	data, err := validator.GenerateConfig("yaml")
	if err != nil {
		t.Fatalf("GenerateConfig failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Generated config is empty")
	}

	// Verify it's valid YAML by loading it
	if err := validator.ValidateConfig(data, "yaml"); err != nil {
		t.Errorf("Generated config is not valid: %v", err)
	}
}

func TestGenerateDefaultScheduler(t *testing.T) {
	validator := cueconfig.NewValidator()
	data, err := validator.GenerateScheduler("yaml")
	if err != nil {
		t.Fatalf("GenerateScheduler failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Generated scheduler config is empty")
	}
}

func TestCacheDuration(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `cache:
  cache_duration: "2h"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	duration := CacheDuration(cfg)
	if duration.Hours() != 2 {
		t.Errorf("Expected cache duration 2h, got %v", duration)
	}
}

func TestMaintenanceConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `maintenance:
  cleanup_enabled: true
  history_retention: "336h"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if !MaintenanceCleanupEnabled(cfg) {
		t.Error("Expected cleanup_enabled to be true")
	}

	retention := MaintenanceHistoryRetention(cfg)
	if retention.Hours() != 336 {
		t.Errorf("Expected history retention 336h, got %v", retention)
	}
}

func TestAPIConfig_DefaultsWhenOmitted(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `tunarr:
  url: "http://localhost:8000"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got := APIListen(cfg); got != ":8484" {
		t.Errorf("Expected CUE-schema default api.listen ':8484', got %q", got)
	}
	if got := APIToken(cfg); got != "" {
		t.Errorf("Expected empty api.token when omitted, got %q", got)
	}
	if APIInsecureNoAuth(cfg) {
		t.Error("Expected api.insecure_no_auth to default to false")
	}
}

func TestAPIConfig_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `api:
  listen: "127.0.0.1:9999"
  token: "configured-token-not-from-env-0123"
  insecure_no_auth: true
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if got := APIListen(cfg); got != "127.0.0.1:9999" {
		t.Errorf("Expected api.listen '127.0.0.1:9999', got %q", got)
	}
	if got := APIToken(cfg); got != "configured-token-not-from-env-0123" {
		t.Errorf("Expected api.token from file, got %q", got)
	}
	if !APIInsecureNoAuth(cfg) {
		t.Error("Expected api.insecure_no_auth to be true")
	}
}

// TestAPIToken_EnvWinsOverConfig verifies SCHEDULARR_API_TOKEN always
// overrides the api.token config key, even when the file sets one.
func TestAPIToken_EnvWinsOverConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `api:
  token: "from-config-file-not-the-environment-x"
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	t.Setenv(EnvAPIToken, "from-the-environment-0123456789ab")

	if got := APIToken(cfg); got != "from-the-environment-0123456789ab" {
		t.Errorf("Expected SCHEDULARR_API_TOKEN to win over config, got %q", got)
	}
}
