package jellyfin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestClient_RefreshLiveTVGuide_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: "test-key"})
	err := client.RefreshLiveTVGuide(context.Background())
	if err == nil {
		t.Error("Expected error for 500 response, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "500") {
		t.Errorf("Expected error to mention status 500, got: %v", err)
	}
}

func TestClient_RefreshLiveTVGuide_BadRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad request"))
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	err := client.RefreshLiveTVGuide(context.Background())
	if err == nil {
		t.Error("Expected error for 400 response, got nil")
	}
}

func TestClient_RefreshLiveTVGuide_EmptyURL(t *testing.T) {
	client := NewClient(Config{URL: "", APIKey: "test-key"})
	err := client.RefreshLiveTVGuide(context.Background())
	if err == nil {
		t.Error("Expected error for empty URL, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "empty") {
		t.Errorf("Expected error about empty URL, got: %v", err)
	}
}

func TestClient_RefreshLiveTVGuide_InvalidURL(t *testing.T) {
	client := NewClient(Config{URL: "://invalid-url", APIKey: "test-key"})
	err := client.RefreshLiveTVGuide(context.Background())
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}
}

func TestClient_RefreshLiveTVGuide_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no auth header is sent when API key is empty
		if r.Header.Get("X-Emby-Token") != "" {
			t.Error("Expected no X-Emby-Token header when API key is empty")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL, APIKey: ""})
	if err := client.RefreshLiveTVGuide(context.Background()); err != nil {
		t.Fatalf("RefreshLiveTVGuide returned error: %v", err)
	}
}

func TestClient_RefreshLiveTVGuide_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait for context cancellation
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.RefreshLiveTVGuide(ctx)
	if err == nil {
		t.Error("Expected context cancellation error, got nil")
	}
}

func TestClient_RefreshLiveTVGuide_HTTPStatusOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(Config{URL: server.URL})
	if err := client.RefreshLiveTVGuide(context.Background()); err != nil {
		t.Fatalf("RefreshLiveTVGuide should accept 200 OK, got error: %v", err)
	}
}

func TestClient_RefreshLiveTVGuide_URLTrimming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/LiveTv/RefreshGuide" {
			t.Errorf("Expected correct path, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	// Test with trailing slash in URL - should be trimmed
	client := NewClient(Config{URL: server.URL + "/", APIKey: "test"})
	if err := client.RefreshLiveTVGuide(context.Background()); err != nil {
		t.Fatalf("RefreshLiveTVGuide failed: %v", err)
	}
}
