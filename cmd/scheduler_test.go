package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/geekxflood/schedularr/internal/external/tunarr"
	"github.com/geekxflood/schedularr/internal/scheduler"
	"github.com/spf13/viper"
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

	viper.Reset()
	viper.Set("tunarr.url", server.URL)

	cfg := &scheduler.Config{
		Blocks: []scheduler.Block{
			{Name: "Valid", ChannelID: "channel-1"},
			{Name: "Invalid", ChannelID: "missing"},
		},
	}

	errs := validateChannelIDs(cfg)
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errs))
	}
	if got := errs[0].Error(); got == "" || !strings.Contains(got, "missing") {
		t.Fatalf("expected error to mention missing channel, got %q", got)
	}
}
