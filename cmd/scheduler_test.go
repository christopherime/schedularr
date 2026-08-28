package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christopherime/schedularr/internal/config"
	"github.com/christopherime/schedularr/internal/external/tunarr"
	"github.com/christopherime/schedularr/internal/scheduler"
)

func TestValidateChannelIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/channels" {
			t.Fatalf("expected /api/channels, got %s", r.URL.Path)
		}
		channels := []tunarr.Channel{
			{ID: "channel-1", Name: "One"},
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(channels); err != nil {
			t.Fatalf("failed to encode channels: %v", err)
		}
	}))
	defer server.Close()

	// Create a temp config file with the test server URL
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	configContent := `tunarr:
  url: "` + server.URL + `"
  api_key: "test-key"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0600); err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load the config and set it as the global app config
	cfg, err := config.Load(configFile)
	if err != nil {
		t.Fatalf("Failed to load test config: %v", err)
	}

	// Set the global appConfig for this test
	oldConfig := appConfig
	appConfig = cfg
	defer func() { appConfig = oldConfig }()

	schedCfg := &scheduler.Config{
		Blocks: []scheduler.Block{
			{Name: "Valid", ChannelID: "channel-1"},
			{Name: "Invalid", ChannelID: "missing"},
		},
	}

	errs := validateChannelIDs(schedCfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if got := errs[0].Error(); got == "" || !strings.Contains(got, "missing") {
		t.Fatalf("expected error to mention missing channel, got %q", got)
	}
}
