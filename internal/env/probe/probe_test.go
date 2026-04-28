package probe

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestParseFirstVersionToken locks the version-extraction logic
// across the various `--version` output styles probes encounter.
func TestParseFirstVersionToken(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"git version 2.43.0", "2.43.0"},
		{"npm 10.2.4", "10.2.4"},
		{"cargo 1.74.0 (...)", "1.74.0"},
		{"Python 3.11.5", "3.11.5"},
		{"go version go1.22.0 linux/amd64", "1.22.0"},
		{"", ""},
		{"no numbers here", ""},
	}
	for _, c := range cases {
		got := parseFirstVersionToken(c.in)
		if got != c.want {
			t.Errorf("parse(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestProbeProjectFiles covers manifest discovery in a temp dir.
func TestProbeProjectFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"pyproject.toml", "package.json", "Cargo.toml"} {
		_ = os.WriteFile(filepath.Join(dir, name), []byte("stub"), 0o644)
	}
	got := probeProjectFiles(dir)
	for _, name := range []string{"pyproject.toml", "package.json", "Cargo.toml"} {
		if _, ok := got[name]; !ok {
			t.Errorf("expected %s in result", name)
		}
	}
	if _, ok := got["nonexistent.toml"]; ok {
		t.Errorf("non-existent file should not be in result")
	}
}

// TestProbeGitRepoState covers the three git-state outcomes.
func TestProbeGitRepoState(t *testing.T) {
	dir := t.TempDir()
	state := probeGitRepoState(dir)
	if state != "not_initialized" {
		t.Errorf("fresh tmpdir should be not_initialized; got %q", state)
	}
	// Mark it as initialised by creating .git.
	_ = os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	state = probeGitRepoState(dir)
	// With .git/ but no commits we expect either no_commits (if git
	// is on PATH) or unknown (no git binary). Anything except
	// not_initialized is acceptable.
	if state == "not_initialized" {
		t.Errorf(".git/ present should not be not_initialized")
	}
}

// TestRun_BasicShape exercises the top-level entry point and
// asserts the output has at least the OS field populated.
func TestRun_BasicShape(t *testing.T) {
	dir := t.TempDir()
	facts := Run(Options{RepoRoot: dir, ProbeNetwork: false})
	if facts == nil {
		t.Fatal("Run returned nil")
	}
	if facts.OS == "" {
		t.Errorf("OS should be populated; got %+v", facts)
	}
	if facts.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", facts.OS, runtime.GOOS)
	}
	if facts.ProbeDurationMs == 0 {
		t.Errorf("ProbeDurationMs not measured")
	}
}
