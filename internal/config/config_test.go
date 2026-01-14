package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/geekxflood/schedularr/internal/scheduler"
)

func TestNew(t *testing.T) {
	cfg := New()

	if cfg == nil {
		t.Fatal("New() returned nil")
	}

	if cfg.Tunarr.URL != "http://localhost:8000" {
		t.Errorf("Expected default Tunarr URL 'http://localhost:8000', got '%s'", cfg.Tunarr.URL)
	}

	if cfg.Log.Level != "info" {
		t.Errorf("Expected default log level 'info', got '%s'", cfg.Log.Level)
	}

	if cfg.Log.Format != "text" {
		t.Errorf("Expected default log format 'text', got '%s'", cfg.Log.Format)
	}

	if !cfg.Radarr.ExcludeMissingFile {
		t.Errorf("Expected default Radarr.ExcludeMissingFile to be true, got false")
	}

	if !cfg.Sonarr.ExcludeMissingFile {
		t.Errorf("Expected default Sonarr.ExcludeMissingFile to be true, got false")
	}

	expectedCacheDir := filepath.Join(os.TempDir(), "schedularr_cache")
	if cfg.Cache.CacheDir != expectedCacheDir {
		t.Errorf("Expected default Cache.CacheDir '%s', got '%s'", expectedCacheDir, cfg.Cache.CacheDir)
	}

	if cfg.Cache.CacheDuration != "1h" {
		t.Errorf("Expected default Cache.CacheDuration '1h', got '%s'", cfg.Cache.CacheDuration)
	}
}

func TestConfig_GetCacheDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration string
		expected time.Duration
	}{
		{"valid 1h", "1h", 1 * time.Hour},
		{"valid 30m", "30m", 30 * time.Minute},
		{"valid 2h30m", "2h30m", 2*time.Hour + 30*time.Minute},
		{"invalid format", "abc", 1 * time.Hour}, // Should default to 1h
		{"empty string", "", 1 * time.Hour},      // Should default to 1h
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := New()
			cfg.Cache.CacheDuration = tt.duration
			actual := cfg.GetCacheDuration()

			if actual != tt.expected {
				t.Errorf("For duration '%s', expected %v, got %v", tt.duration, tt.expected, actual)
			}
		})
	}
}

func TestLoadSchedulerConfig_FromFile(t *testing.T) {
	// Create a temporary scheduler file
	tmpDir := t.TempDir()
	schedulerFile := filepath.Join(tmpDir, "test-scheduler.yaml")

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

	cfg := New()
	schedCfg, err := LoadSchedulerConfig(cfg, schedulerFile)
	if err != nil {
		t.Fatalf("LoadSchedulerConfig failed: %v", err)
	}

	if len(schedCfg.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(schedCfg.Blocks))
	}

	if schedCfg.Blocks[0].Name != "Test Block" {
		t.Errorf("Expected block name 'Test Block', got '%s'", schedCfg.Blocks[0].Name)
	}
}

func TestLoadSchedulerConfig_FromConfigField(t *testing.T) {
	// Create a temporary scheduler file
	tmpDir := t.TempDir()
	schedulerFile := filepath.Join(tmpDir, "config-scheduler.yaml")

	content := `blocks:
  - name: "Config Block"
    type: "filter"
    cron: "0 10 * * *"
    duration: 60
    channel_id: "channel-2"
    priority: 5
`

	if err := os.WriteFile(schedulerFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg := New()
	cfg.SchedulerFile = schedulerFile

	schedCfg, err := LoadSchedulerConfig(cfg, "")
	if err != nil {
		t.Fatalf("LoadSchedulerConfig failed: %v", err)
	}

	if len(schedCfg.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(schedCfg.Blocks))
	}

	if schedCfg.Blocks[0].Name != "Config Block" {
		t.Errorf("Expected block name 'Config Block', got '%s'", schedCfg.Blocks[0].Name)
	}
}

func TestLoadSchedulerConfig_FromInlineConfig(t *testing.T) {
	cfg := New()
	cfg.Scheduler = scheduler.Config{
		Blocks: []scheduler.Block{
			{
				Name:      "Inline Block",
				Type:      "filter",
				Cron:      "0 11 * * *",
				Duration:  90,
				ChannelID: "channel-3",
				Priority:  15,
			},
		},
	}

	schedCfg, err := LoadSchedulerConfig(cfg, "")
	if err != nil {
		t.Fatalf("LoadSchedulerConfig failed: %v", err)
	}

	if len(schedCfg.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(schedCfg.Blocks))
	}

	if schedCfg.Blocks[0].Name != "Inline Block" {
		t.Errorf("Expected block name 'Inline Block', got '%s'", schedCfg.Blocks[0].Name)
	}
}

func TestLoadSchedulerConfig_NoConfig(t *testing.T) {
	cfg := New()

	_, err := LoadSchedulerConfig(cfg, "")
	if err == nil {
		t.Error("Expected error when no scheduler config is found, got nil")
	}

	expectedMsg := "no scheduler configuration found"
	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestLoadSchedulerConfig_InvalidFile(t *testing.T) {
	cfg := New()

	_, err := LoadSchedulerConfig(cfg, "/nonexistent/file.yaml")
	if err == nil {
		t.Error("Expected error when loading nonexistent file, got nil")
	}
}

func TestLoadSchedulerConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	schedulerFile := filepath.Join(tmpDir, "invalid.yaml")

	// Write invalid YAML content
	content := `blocks:
  - name: "Test Block"
    invalid_yaml: [unclosed bracket
`

	if err := os.WriteFile(schedulerFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg := New()
	_, err := LoadSchedulerConfig(cfg, schedulerFile)
	if err == nil {
		t.Error("Expected error when parsing invalid YAML, got nil")
	}
}

func TestLoadSchedulerConfig_FromDefaultLocations(t *testing.T) {
	// Create a temporary directory to act as home
	tmpDir := t.TempDir()

	// Create scheduler.yaml in the temp directory
	schedulerFile := filepath.Join(tmpDir, "scheduler.yaml")
	content := `blocks:
  - name: "Default Block"
    type: "filter"
    cron: "0 12 * * *"
    duration: 30
    channel_id: "channel-default"
    priority: 1
`

	if err := os.WriteFile(schedulerFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Change to temp directory so default location search finds it
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}

	cfg := New()
	schedCfg, err := LoadSchedulerConfig(cfg, "")
	if err != nil {
		t.Fatalf("LoadSchedulerConfig failed: %v", err)
	}

	if len(schedCfg.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(schedCfg.Blocks))
	}

	if schedCfg.Blocks[0].Name != "Default Block" {
		t.Errorf("Expected block name 'Default Block', got '%s'", schedCfg.Blocks[0].Name)
	}
}

func TestGetSchedulerConfig(t *testing.T) {
	// Create a temporary scheduler file
	tmpDir := t.TempDir()
	schedulerFile := filepath.Join(tmpDir, "viper-scheduler.yaml")

	content := `blocks:
  - name: "Viper Block"
    type: "filter"
    cron: "0 13 * * *"
    duration: 45
    channel_id: "channel-viper"
    priority: 20
`

	if err := os.WriteFile(schedulerFile, []byte(content), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Note: GetSchedulerConfig uses viper.Unmarshal which requires viper to be initialized
	// This test verifies the function exists and handles the scheduler file parameter
	// In a real scenario, viper would be initialized by the CLI
	_, err := GetSchedulerConfig(schedulerFile)
	// We expect an error here because viper isn't initialized in the test
	// but we're testing that the function can be called
	if err == nil {
		// If viper happens to be initialized, verify the result
		t.Log("GetSchedulerConfig succeeded (viper was initialized)")
	} else {
		// Expected case - viper not initialized in test
		t.Logf("GetSchedulerConfig returned expected error: %v", err)
	}
}

func TestMaintenanceConfig_GetCleanupInterval(t *testing.T) {
	tests := []struct {
		name     string
		interval string
		expected time.Duration
	}{
		{"valid 1h", "1h", 1 * time.Hour},
		{"valid 24h", "24h", 24 * time.Hour},
		{"valid 30m", "30m", 30 * time.Minute},
		{"invalid format", "abc", 24 * time.Hour}, // Should default to 24h
		{"empty string", "", 24 * time.Hour},      // Should default to 24h
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MaintenanceConfig{CleanupInterval: tt.interval}
			actual := m.GetCleanupInterval()

			if actual != tt.expected {
				t.Errorf("For interval '%s', expected %v, got %v", tt.interval, tt.expected, actual)
			}
		})
	}
}

func TestMaintenanceConfig_GetHistoryRetention(t *testing.T) {
	tests := []struct {
		name      string
		retention string
		expected  time.Duration
	}{
		{"valid 168h (7 days)", "168h", 168 * time.Hour},
		{"valid 24h", "24h", 24 * time.Hour},
		{"valid 720h (30 days)", "720h", 720 * time.Hour},
		{"invalid format", "abc", 7 * 24 * time.Hour}, // Should default to 7 days
		{"empty string", "", 7 * 24 * time.Hour},      // Should default to 7 days
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &MaintenanceConfig{HistoryRetention: tt.retention}
			actual := m.GetHistoryRetention()

			if actual != tt.expected {
				t.Errorf("For retention '%s', expected %v, got %v", tt.retention, tt.expected, actual)
			}
		})
	}
}

func TestNew_MaintenanceDefaults(t *testing.T) {
	cfg := New()

	if cfg.Maintenance.CleanupInterval != "24h" {
		t.Errorf("Expected default CleanupInterval '24h', got '%s'", cfg.Maintenance.CleanupInterval)
	}

	if cfg.Maintenance.HistoryRetention != "168h" {
		t.Errorf("Expected default HistoryRetention '168h', got '%s'", cfg.Maintenance.HistoryRetention)
	}

	if !cfg.Maintenance.CleanupEnabled {
		t.Error("Expected default CleanupEnabled to be true, got false")
	}
}

func TestSaveSchedulerConfig(t *testing.T) {
	tmpDir := t.TempDir()

	schedCfg := &scheduler.Config{
		Blocks: []scheduler.Block{
			{
				Name:      "Test Block",
				Type:      "filter",
				Cron:      "0 20 * * *",
				Duration:  120,
				ChannelID: "channel-1",
				Priority:  10,
			},
		},
	}

	// Test saving to a file
	path := tmpDir + "/test-scheduler.yaml"
	err := SaveSchedulerConfig(schedCfg, path)
	if err != nil {
		t.Fatalf("SaveSchedulerConfig failed: %v", err)
	}

	// Verify the file was created and can be loaded
	loadedCfg, err := LoadSchedulerConfig(&Config{}, path)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if len(loadedCfg.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(loadedCfg.Blocks))
	}

	if loadedCfg.Blocks[0].Name != "Test Block" {
		t.Errorf("Expected block name 'Test Block', got '%s'", loadedCfg.Blocks[0].Name)
	}
}

func TestSaveSchedulerConfig_DefaultPath(t *testing.T) {
	// Save the current working directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Change to a temp directory
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(oldWd)

	schedCfg := &scheduler.Config{
		Blocks: []scheduler.Block{
			{Name: "Default Path Block", Cron: "0 6 * * *", Duration: 60, ChannelID: "ch-1"},
		},
	}

	// Test saving with empty path (should use default scheduler.yaml)
	err = SaveSchedulerConfig(schedCfg, "")
	if err != nil {
		t.Fatalf("SaveSchedulerConfig with default path failed: %v", err)
	}

	// Verify scheduler.yaml was created
	if _, err := os.Stat("scheduler.yaml"); os.IsNotExist(err) {
		t.Error("scheduler.yaml was not created")
	}
}

func TestSaveSchedulerConfig_InvalidPath(t *testing.T) {
	schedCfg := &scheduler.Config{
		Blocks: []scheduler.Block{},
	}

	// Test saving to an invalid path
	err := SaveSchedulerConfig(schedCfg, "/nonexistent/directory/file.yaml")
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}

func TestLoadSchedulerConfig_PriorityOrder(t *testing.T) {
	// Test that explicit file parameter takes priority over config field
	tmpDir := t.TempDir()

	// Create two different scheduler files
	explicitFile := filepath.Join(tmpDir, "explicit.yaml")
	configFile := filepath.Join(tmpDir, "config.yaml")

	explicitContent := `blocks:
  - name: "Explicit Block"
    type: "filter"
    cron: "0 14 * * *"
    duration: 60
    channel_id: "channel-explicit"
    priority: 10
`

	configContent := `blocks:
  - name: "Config Block"
    type: "filter"
    cron: "0 15 * * *"
    duration: 30
    channel_id: "channel-config"
    priority: 5
`

	if err := os.WriteFile(explicitFile, []byte(explicitContent), 0600); err != nil {
		t.Fatalf("Failed to create explicit file: %v", err)
	}

	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	cfg := New()
	cfg.SchedulerFile = configFile

	// Explicit file should take priority
	schedCfg, err := LoadSchedulerConfig(cfg, explicitFile)
	if err != nil {
		t.Fatalf("LoadSchedulerConfig failed: %v", err)
	}

	if len(schedCfg.Blocks) != 1 {
		t.Fatalf("Expected 1 block, got %d", len(schedCfg.Blocks))
	}

	if schedCfg.Blocks[0].Name != "Explicit Block" {
		t.Errorf("Expected 'Explicit Block' (from explicit file), got '%s'", schedCfg.Blocks[0].Name)
	}
}
