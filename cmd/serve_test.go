package cmd

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/api"
	"github.com/christopherime/schedularr/internal/service"
	"github.com/christopherime/schedularr/internal/store"
)

// fakeScheduleRunner is a minimal service.ScheduleRunner double for
// TestServeUntil_HealthzThenCleanShutdown: the smoke test only needs
// runCronLoop's first immediate tick to succeed harmlessly, not to
// exercise real schedule generation (that's internal/service's own test
// suite's job).
type fakeScheduleRunner struct{}

func (fakeScheduleRunner) Run(_ context.Context, o service.Options) (*service.Result, error) {
	return &service.Result{Applied: o.Apply}, nil
}

// TestServeUntil_HealthzThenCleanShutdown is the serve command's smoke
// test (task brief step 3): start the server on an OS-assigned port
// (":0"), confirm /healthz answers 200, cancel the context (the same
// signal serveUntil reacts to from signal.NotifyContext in runServe), and
// confirm serveUntil returns cleanly within a bounded wait -- proving both
// the HTTP server and the cron loop shut down instead of leaking
// goroutines or hanging.
func TestServeUntil_HealthzThenCleanShutdown(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	router, err := api.NewRouter(
		api.Config{InsecureNoAuth: true},
		api.Deps{Store: s, Logger: slog.Default(), Version: "test"},
	)
	if err != nil {
		t.Fatalf("failed to build router: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on :0: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- serveUntil(ctx, ln, router, fakeScheduleRunner{}, slog.Default())
	}()

	addr := "http://" + ln.Addr().String() + "/healthz"
	var resp *http.Response
	var reqErr error
	for range 50 {
		resp, reqErr = http.Get(addr) //nolint:gosec,noctx // test-only, fixed local address
		if reqErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if reqErr != nil {
		t.Fatalf("failed to reach /healthz: %v", reqErr)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from /healthz, got %d", resp.StatusCode)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serveUntil returned error after shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveUntil did not return within 5s of context cancellation")
	}
}
