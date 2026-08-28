package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/christopherime/schedularr/internal/scheduler"
	"github.com/christopherime/schedularr/internal/store"
)

// TestHandleScheduleOutput_RequiresYesToApply verifies that apply is refused
// non-interactively unless --yes is set. This is the replacement for the
// removed charmbracelet/huh confirmation prompt: since no interactive code
// remains, an explicit --yes flag is now mandatory before any mutating
// apply happens.
func TestHandleScheduleOutput_RequiresYesToApply(t *testing.T) {
	t.Run("refuses apply without --yes", func(t *testing.T) {
		origAssumeYes := assumeYes
		assumeYes = false
		defer func() { assumeYes = origAssumeYes }()

		err := handleScheduleOutput(nil, nil, nil, nil, scheduleOutputOptions{apply: true, dryRun: false})
		if err == nil {
			t.Fatal("expected error when applying without --yes, got nil")
		}
		const want = "refusing to apply without --yes (interactive prompts were removed)"
		if err.Error() != want {
			t.Fatalf("unexpected error message: got %q, want %q", err.Error(), want)
		}
	})

	t.Run("dry-run does not require --yes", func(t *testing.T) {
		origAssumeYes := assumeYes
		assumeYes = false
		defer func() { assumeYes = origAssumeYes }()

		err := handleScheduleOutput(nil, nil, nil, nil, scheduleOutputOptions{apply: true, dryRun: true})
		if err != nil {
			t.Fatalf("dry-run should not require --yes, got error: %v", err)
		}
	})
}

// TestColorEnabled verifies the ANSI style helper's TTY/NO_COLOR detection,
// which replaces lipgloss's own auto-detection (removed with the
// charmbracelet purge). Both cases matter for this task's --yes scripting
// contract: piped/cron/CI invocations of `generate --apply --yes` must not
// emit raw escape sequences into logs or downstream tooling.
func TestColorEnabled(t *testing.T) {
	t.Run("NO_COLOR set disables color", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")

		if colorEnabled() {
			t.Fatal("expected colorEnabled() to be false when NO_COLOR is set")
		}
		if got := errorStyle.Render("x"); got != "x" {
			t.Fatalf("Render with NO_COLOR set: got %q, want unstyled %q", got, "x")
		}
	})

	t.Run("non-TTY stdout disables color (covers piped/cron/CI output)", func(t *testing.T) {
		unsetEnv(t, "NO_COLOR")

		// go test's stdout is captured/piped, not a character device, so
		// this exercises the same branch a piped or cron invocation of
		// schedularr would hit even with NO_COLOR unset.
		fi, err := os.Stdout.Stat()
		if err != nil {
			t.Fatalf("os.Stdout.Stat: %v", err)
		}
		if fi.Mode()&os.ModeCharDevice != 0 {
			t.Skip("test process stdout is a character device; non-TTY branch not exercised in this environment")
		}

		if colorEnabled() {
			t.Fatal("expected colorEnabled() to be false when stdout is not a TTY")
		}
		if got := errorStyle.Render("x"); got != "x" {
			t.Fatalf("Render on non-TTY stdout: got %q, want unstyled %q", got, "x")
		}
	})
}

// TestLoadActiveBlocks verifies that loadActiveBlocks returns only the
// Specs of enabled block records from the store -- disabled blocks are the
// mechanism for keeping a block defined but out of schedule generation, so
// the engine must never see one.
func TestLoadActiveBlocks(t *testing.T) {
	s, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx := context.Background()

	enabled := &store.BlockRecord{
		ID:      "enabled-1",
		Name:    "Enabled Block",
		Enabled: true,
		Spec: scheduler.Block{
			Name:      "Enabled Block",
			Cron:      "0 6 * * *",
			Duration:  60,
			ChannelID: "channel-1",
		},
	}
	disabled := &store.BlockRecord{
		ID:      "disabled-1",
		Name:    "Disabled Block",
		Enabled: false,
		Spec: scheduler.Block{
			Name:      "Disabled Block",
			Cron:      "0 7 * * *",
			Duration:  30,
			ChannelID: "channel-2",
		},
	}

	if err := s.CreateBlock(ctx, enabled); err != nil {
		t.Fatalf("failed to create enabled block: %v", err)
	}
	if err := s.CreateBlock(ctx, disabled); err != nil {
		t.Fatalf("failed to create disabled block: %v", err)
	}

	blocks, err := loadActiveBlocks(ctx, s)
	if err != nil {
		t.Fatalf("loadActiveBlocks failed: %v", err)
	}

	if len(blocks) != 1 {
		t.Fatalf("expected 1 active block, got %d: %+v", len(blocks), blocks)
	}
	if blocks[0].Name != "Enabled Block" {
		t.Errorf("expected active block named 'Enabled Block', got %q", blocks[0].Name)
	}
}

// unsetEnv removes key from the environment for the duration of the test,
// restoring its previous value (or leaving it unset) on cleanup.
func unsetEnv(t *testing.T, key string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("failed to unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, old)
		}
	})
}
