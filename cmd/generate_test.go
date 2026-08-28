package cmd

import (
	"os"
	"testing"
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
