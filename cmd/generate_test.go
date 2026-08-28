package cmd

import "testing"

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
