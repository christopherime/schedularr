package cmd

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/christopherime/schedularr/internal/api"
	"github.com/christopherime/schedularr/internal/config"
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
		// A 1h interval is arbitrary here -- the test cancels ctx well
		// before that, so the ticker never actually fires; only the
		// immediate startup tick (runCronLoop's unconditional first call)
		// runs.
		done <- serveUntil(ctx, serveParams{
			ln:           ln,
			handler:      router,
			runner:       fakeScheduleRunner{},
			cronInterval: time.Hour,
			logger:       slog.Default(),
		})
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

// loadTestConfig writes content to a temp config.yaml and loads it,
// failing the test on any error. Shared by the resolveCronInterval
// precedence tests below.
func loadTestConfig(t *testing.T, content string) *config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("failed to load test config: %v", err)
	}
	return cfg
}

// TestResolveCronInterval_FlagWinsWhenChanged verifies --interval/-i wins
// over cron_interval when explicitly passed, even though the config file
// sets a different value.
func TestResolveCronInterval_FlagWinsWhenChanged(t *testing.T) {
	cfg := loadTestConfig(t, "cron_interval: \"2h\"\n")

	got := resolveCronInterval(true, 30*time.Minute, cfg)
	if got != 30*time.Minute {
		t.Errorf("expected flag value 30m to win, got %v", got)
	}
}

// TestResolveCronInterval_FallsBackToConfigWhenFlagNotChanged verifies the
// cron_interval config key applies when --interval wasn't explicitly
// passed -- the flag's own default value (whatever DurationVarP was given)
// must be ignored in that case, not treated as an explicit choice.
func TestResolveCronInterval_FallsBackToConfigWhenFlagNotChanged(t *testing.T) {
	cfg := loadTestConfig(t, "cron_interval: \"2h\"\n")

	got := resolveCronInterval(false, 999*time.Hour, cfg)
	if got != 2*time.Hour {
		t.Errorf("expected config value 2h when flag unchanged, got %v", got)
	}
}

// TestResolveCronInterval_DefaultsTo6hWhenNothingSet verifies the full
// flag > config > 6h-default chain bottoms out at 6h when neither the flag
// nor the config file set anything.
func TestResolveCronInterval_DefaultsTo6hWhenNothingSet(t *testing.T) {
	cfg := loadTestConfig(t, "tunarr:\n  url: \"http://localhost:8000\"\n")

	got := resolveCronInterval(false, 0, cfg)
	if got != 6*time.Hour {
		t.Errorf("expected 6h default when nothing is set, got %v", got)
	}
}
