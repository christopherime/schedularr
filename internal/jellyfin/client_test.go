package jellyfin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_RefreshLiveTVGuide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/LiveTv/RefreshGuide" {
			t.Errorf("expected /LiveTv/RefreshGuide path, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Emby-Token") != "jellyfin-key" {
			t.Errorf("expected X-Emby-Token header, got %s", r.Header.Get("X-Emby-Token"))
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "jellyfin-key"})
	if err := client.RefreshLiveTVGuide(context.Background()); err != nil {
		t.Fatalf("RefreshLiveTVGuide returned error: %v", err)
	}
}
