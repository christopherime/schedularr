package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/geekxflood/schedularr/internal/cueconfig"
	"github.com/geekxflood/schedularr/internal/scheduler"
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

func TestJellyfinConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `jellyfin:
  url: "http://jellyfin:8096"
  api_key: "jellyfin-key"
  user_id: "user-123"
  sync_live_tv: true
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	jellyfinCfg := JellyfinConfig(cfg)

	if jellyfinCfg.URL != "http://jellyfin:8096" {
		t.Errorf("Expected Jellyfin URL 'http://jellyfin:8096', got '%s'", jellyfinCfg.URL)
	}

	if !jellyfinCfg.SyncLiveTV {
		t.Error("Expected SyncLiveTV to be true")
	}
}

func TestRadarrConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `radarr:
  url: "http://radarr:7878"
  api_key: "radarr-key"
  exclude_missing_file: true
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	radarrCfg := RadarrConfig(cfg)

	if radarrCfg.URL != "http://radarr:7878" {
		t.Errorf("Expected Radarr URL 'http://radarr:7878', got '%s'", radarrCfg.URL)
	}

	if !radarrCfg.ExcludeMissingFile {
		t.Error("Expected ExcludeMissingFile to be true")
	}
}

func TestSonarrConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	content := `sonarr:
  url: "http://sonarr:8989"
  api_key: "sonarr-key"
  exclude_missing_file: false
`

	if err := os.WriteFile(configFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	sonarrCfg := SonarrConfig(cfg)

	if sonarrCfg.URL != "http://sonarr:8989" {
		t.Errorf("Expected Sonarr URL 'http://sonarr:8989', got '%s'", sonarrCfg.URL)
	}

	if sonarrCfg.ExcludeMissingFile {
		t.Error("Expected ExcludeMissingFile to be false")
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

func TestLoadScheduler(t *testing.T) {
	tmpDir := t.TempDir()
	schedulerFile := filepath.Join(tmpDir, "scheduler.yaml")

	content := `blocks:
  - name: "Test Block"
    type: "filter"
    cron: "0 9 * * *"
    duration: 120
    channel_id: "channel-1"
    priority: 10
    filter:
      genres: ["Comedy"]
`

	if err := os.WriteFile(schedulerFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	schedCfg, err := LoadScheduler(schedulerFile)
	if err != nil {
		t.Fatalf("LoadScheduler failed: %v", err)
	}

	blocks := schedCfg.GetBlocks()
	if len(blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(blocks))
	}

	if name := blocks[0]["name"].(string); name != "Test Block" {
		t.Errorf("Expected block name 'Test Block', got '%s'", name)
	}
}

func TestSchedulerBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	schedulerFile := filepath.Join(tmpDir, "scheduler.yaml")

	content := `blocks:
  - name: "Morning Block"
    type: "filter"
    cron: "0 6 * * *"
    duration: 180
    channel_id: "ch-1"
    priority: 5
    filter:
      genres: ["Animation"]
      min_duration: 20
      max_duration: 30
`

	if err := os.WriteFile(schedulerFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	schedCfg, err := LoadScheduler(schedulerFile)
	if err != nil {
		t.Fatalf("LoadScheduler failed: %v", err)
	}

	blocks := SchedulerBlocks(schedCfg)
	if len(blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(blocks))
	}

	block := blocks[0]
	if block.Name != "Morning Block" {
		t.Errorf("Expected block name 'Morning Block', got '%s'", block.Name)
	}

	if block.Duration != 180 {
		t.Errorf("Expected duration 180, got %d", block.Duration)
	}

	if block.Priority != 5 {
		t.Errorf("Expected priority 5, got %d", block.Priority)
	}

	if len(block.Filter.Genres) != 1 || block.Filter.Genres[0] != "Animation" {
		t.Errorf("Expected filter genres [Animation], got %v", block.Filter.Genres)
	}
}

func TestFindSchedulerConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config file with scheduler_file reference
	configFile := filepath.Join(tmpDir, "config.yaml")
	schedulerFile := filepath.Join(tmpDir, "my-scheduler.yaml")

	configContent := `scheduler_file: "` + schedulerFile + `"
`
	schedulerContent := `blocks:
  - name: "Found Block"
    cron: "0 8 * * *"
    duration: 60
    channel_id: "ch-2"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	if err := os.WriteFile(schedulerFile, []byte(schedulerContent), 0600); err != nil {
		t.Fatalf("Failed to create scheduler file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	schedCfg, err := FindSchedulerConfig(cfg, "")
	if err != nil {
		t.Fatalf("FindSchedulerConfig failed: %v", err)
	}

	blocks := schedCfg.GetBlocks()
	if len(blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(blocks))
	}

	if name := blocks[0]["name"].(string); name != "Found Block" {
		t.Errorf("Expected 'Found Block', got '%s'", name)
	}
}

func TestFindSchedulerConfig_ExplicitFile(t *testing.T) {
	tmpDir := t.TempDir()

	configFile := filepath.Join(tmpDir, "config.yaml")
	schedulerFile := filepath.Join(tmpDir, "explicit-scheduler.yaml")

	configContent := `tunarr:
  url: "http://localhost:8000"
`
	schedulerContent := `blocks:
  - name: "Explicit Block"
    cron: "0 9 * * *"
    duration: 90
    channel_id: "ch-3"
`

	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	if err := os.WriteFile(schedulerFile, []byte(schedulerContent), 0600); err != nil {
		t.Fatalf("Failed to create scheduler file: %v", err)
	}

	cfg, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Explicit file should be used
	schedCfg, err := FindSchedulerConfig(cfg, schedulerFile)
	if err != nil {
		t.Fatalf("FindSchedulerConfig failed: %v", err)
	}

	blocks := schedCfg.GetBlocks()
	if len(blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(blocks))
	}

	if name := blocks[0]["name"].(string); name != "Explicit Block" {
		t.Errorf("Expected 'Explicit Block', got '%s'", name)
	}
}

func TestSaveSchedulerBlocks(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output-scheduler.yaml")

	blocks := []scheduler.Block{
		{
			Name:      "Saved Block",
			Type:      "filter",
			Cron:      "0 10 * * *",
			Duration:  120,
			ChannelID: "ch-save",
			Priority:  10,
			Filter: scheduler.Filter{
				Genres: []string{"Drama"},
			},
		},
	}

	if err := SaveSchedulerBlocks(blocks, outputPath); err != nil {
		t.Fatalf("SaveSchedulerBlocks failed: %v", err)
	}

	// Verify by loading the saved file
	schedCfg, err := LoadScheduler(outputPath)
	if err != nil {
		t.Fatalf("Failed to load saved scheduler: %v", err)
	}

	loadedBlocks := SchedulerBlocks(schedCfg)
	if len(loadedBlocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(loadedBlocks))
	}

	if loadedBlocks[0].Name != "Saved Block" {
		t.Errorf("Expected 'Saved Block', got '%s'", loadedBlocks[0].Name)
	}

	if loadedBlocks[0].Duration != 120 {
		t.Errorf("Expected duration 120, got %d", loadedBlocks[0].Duration)
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
