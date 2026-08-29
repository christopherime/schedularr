package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeTempConfig creates an empty file at dir/name and returns its path.
// FindConfigFile only checks existence (os.Stat), so content is irrelevant.
func writeTempConfig(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("tunarr:\n  url: \"\"\n"), 0600); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	return path
}

// TestFindConfigFile_EnvPathAbsolute is the baseline: SCHEDULARR_CONFIG set
// to a real, existing absolute path resolves unchanged by the
// filepath.Clean() call FindConfigFile now applies (an already-clean
// absolute path is a no-op for Clean).
func TestFindConfigFile_EnvPathAbsolute(t *testing.T) {
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "config.yaml")

	t.Setenv(EnvConfigPath, path)

	got := FindConfigFile("")
	if got != path {
		t.Errorf("FindConfigFile(\"\") = %q, want %q", got, path)
	}
}

// TestFindConfigFile_EnvPathRelativeDotSlash confirms a relative path with a
// leading "./" -- which filepath.Clean() normalizes away ("./config.yaml"
// becomes "config.yaml") -- still resolves to the same file. Clean() only
// changes the string form of the path, never what it points at.
func TestFindConfigFile_EnvPathRelativeDotSlash(t *testing.T) {
	dir := t.TempDir()
	writeTempConfig(t, dir, "config.yaml")
	t.Chdir(dir)

	t.Setenv(EnvConfigPath, "./config.yaml")

	got := FindConfigFile("")
	if got != "config.yaml" {
		t.Errorf("FindConfigFile(\"\") = %q, want %q (Clean-normalized)", got, "config.yaml")
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("resolved path %q does not stat: %v", got, err)
	}
}

// TestFindConfigFile_EnvPathTrailingSlash is the case the code review
// specifically asked to confirm. Before FindConfigFile applied
// filepath.Clean(), a SCHEDULARR_CONFIG value with an accidental trailing
// slash (e.g. copy-pasted from a directory listing, or produced by shell
// tab-completion) would fail to resolve: os.Stat("<file>/") returns "not a
// directory" for a regular file on both Linux and macOS, since a trailing
// slash asserts directory semantics. filepath.Clean() strips the trailing
// slash before the Stat call, so this now resolves -- a genuine behavior
// improvement from the fix, not just a neutral no-op.
func TestFindConfigFile_EnvPathTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "config.yaml")

	t.Setenv(EnvConfigPath, path+"/")

	got := FindConfigFile("")
	if got != path {
		t.Errorf("FindConfigFile(\"\") = %q, want %q (trailing slash stripped)", got, path)
	}
}

// TestFindConfigFile_EnvPathDoesNotExist confirms the negative case is
// unaffected: a SCHEDULARR_CONFIG value that doesn't exist on disk still
// returns "", whether or not filepath.Clean() changes its string form.
func TestFindConfigFile_EnvPathDoesNotExist(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.yaml")

	t.Setenv(EnvConfigPath, missing+"/")

	if got := FindConfigFile(""); got != "" {
		t.Errorf("FindConfigFile(\"\") = %q, want \"\"", got)
	}
}

// TestFindConfigFile_FlagPathTrailingSlash mirrors the env-path trailing-
// slash case for the --config flag value (priority 1), which FindConfigFile
// now also runs through filepath.Clean() for the same reason and the same
// benefit -- symmetry with the env-path treatment above, not because gosec
// ever flagged the flag-sourced path (it never did: gosec's G703 taint
// tracker only treats os.Getenv/os.Args as tainted sources, not function
// parameters like flagValue).
func TestFindConfigFile_FlagPathTrailingSlash(t *testing.T) {
	dir := t.TempDir()
	path := writeTempConfig(t, dir, "config.yaml")

	got := FindConfigFile(path + "/")
	if got != path {
		t.Errorf("FindConfigFile(%q) = %q, want %q (trailing slash stripped)", path+"/", got, path)
	}
}

// TestFindConfigFile_FlagPathTakesPriorityOverEnv confirms Clean()-ing both
// sources didn't disturb FindConfigFile's existing flag-over-env priority
// order.
func TestFindConfigFile_FlagPathTakesPriorityOverEnv(t *testing.T) {
	dir := t.TempDir()
	flagPath := writeTempConfig(t, dir, "flag-config.yaml")
	envPath := writeTempConfig(t, dir, "env-config.yaml")

	t.Setenv(EnvConfigPath, envPath)

	got := FindConfigFile(flagPath)
	if got != flagPath {
		t.Errorf("FindConfigFile(%q) = %q, want %q (flag must win over env)", flagPath, got, flagPath)
	}
}
