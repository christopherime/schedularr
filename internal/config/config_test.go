package config

import (
	"os"
	"path/filepath"
	"testing"

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

