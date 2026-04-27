package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDetectRepoState_Ready covers the happy path: a freshly inited
// repo with one commit returns RepoReady.
func TestDetectRepoState_Ready(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "x@y")
	mustGit(t, dir, "config", "user.name", "x")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "init")

	state, err := DetectRepoState(dir)
	if err != nil {
		t.Fatalf("DetectRepoState: %v", err)
	}
	if state != RepoReady {
		t.Errorf("state = %v, want RepoReady", state)
	}
	if state.NeedsInit() {
		t.Error("RepoReady should not NeedsInit()")
	}
}

// TestDetectRepoState_NotInitialized covers a bare directory with no
// .git: returns RepoNotInitialized.
func TestDetectRepoState_NotInitialized(t *testing.T) {
	dir := t.TempDir()
	state, err := DetectRepoState(dir)
	if err != nil {
		t.Fatalf("DetectRepoState: %v", err)
	}
	if state != RepoNotInitialized {
		t.Errorf("state = %v, want RepoNotInitialized", state)
	}
	if !state.NeedsInit() {
		t.Error("RepoNotInitialized should NeedsInit()")
	}
}

// TestDetectRepoState_NoCommits covers an inited repo with no
// commits yet: returns RepoNoCommits.
func TestDetectRepoState_NoCommits(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	state, err := DetectRepoState(dir)
	if err != nil {
		t.Fatalf("DetectRepoState: %v", err)
	}
	if state != RepoNoCommits {
		t.Errorf("state = %v, want RepoNoCommits", state)
	}
	if !state.NeedsInit() {
		t.Error("RepoNoCommits should NeedsInit()")
	}
}

// TestDetectRepoState_StatErrors verifies a non-existent path
// returns an error rather than mis-classifying.
func TestDetectRepoState_StatErrors(t *testing.T) {
	_, err := DetectRepoState(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
	if !strings.Contains(err.Error(), "stat") {
		t.Errorf("error should mention stat: %v", err)
	}
}

// TestEnsureInitialCommit_FreshDir runs the full RepoNotInitialized
// → RepoReady transition: git init + identity defaults + initial
// commit. End state probes RepoReady.
func TestEnsureInitialCommit_FreshDir(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureInitialCommit(dir, "codrax: initial commit for plan-1"); err != nil {
		t.Fatalf("EnsureInitialCommit: %v", err)
	}
	state, _ := DetectRepoState(dir)
	if state != RepoReady {
		t.Errorf("after init: state = %v, want RepoReady", state)
	}
	// Confirm identity got set locally so later commits work.
	if !hasGitConfig(dir, "user.email") {
		t.Error("user.email should be set by EnsureInitialCommit")
	}
	if !hasGitConfig(dir, "user.name") {
		t.Error("user.name should be set")
	}
}

// TestEnsureInitialCommit_NoCommits handles the RepoNoCommits state:
// .git exists, just need the commit. Doesn't run `git init` again.
func TestEnsureInitialCommit_NoCommits(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	if err := EnsureInitialCommit(dir, "x"); err != nil {
		t.Fatalf("EnsureInitialCommit: %v", err)
	}
	state, _ := DetectRepoState(dir)
	if state != RepoReady {
		t.Errorf("state = %v, want RepoReady", state)
	}
}

// TestEnsureInitialCommit_Idempotent verifies a second call on a
// RepoReady directory is a no-op success — the previous commit
// remains the HEAD.
func TestEnsureInitialCommit_Idempotent(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureInitialCommit(dir, "first"); err != nil {
		t.Fatalf("first init: %v", err)
	}
	beforeSHA := captureHEAD(t, dir)
	if err := EnsureInitialCommit(dir, "second-noop"); err != nil {
		t.Fatalf("second init: %v", err)
	}
	afterSHA := captureHEAD(t, dir)
	if beforeSHA != afterSHA {
		t.Errorf("idempotent call should not change HEAD; before=%s after=%s",
			beforeSHA, afterSHA)
	}
}

// TestEnsureInitialCommit_PreservesExistingIdentity verifies the
// helper does not overwrite user-configured identity. Real users
// may have already set user.name/email globally OR in the local
// repo; we should respect that.
func TestEnsureInitialCommit_PreservesExistingIdentity(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "existing@example.com")
	mustGit(t, dir, "config", "user.name", "Existing")

	if err := EnsureInitialCommit(dir, "x"); err != nil {
		t.Fatalf("EnsureInitialCommit: %v", err)
	}
	out, err := exec.Command("git", "-C", dir, "config", "user.email").Output()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "existing@example.com" {
		t.Errorf("user.email = %q, want preserved 'existing@example.com'", got)
	}
}

// TestEnsureInitialCommit_AfterReadyIsNoop ensures the worktree
// pre-hook can call EnsureInitialCommit unconditionally without
// risking duplicate commits — the function short-circuits on
// RepoReady.
func TestEnsureInitialCommit_AfterReadyIsNoop(t *testing.T) {
	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "x@y")
	mustGit(t, dir, "config", "user.name", "x")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "real-first-commit")
	mustGit(t, dir, "commit", "--allow-empty", "-m", "real-second-commit")

	if err := EnsureInitialCommit(dir, "should be skipped"); err != nil {
		t.Fatalf("EnsureInitialCommit on ready repo: %v", err)
	}
	// Verify HEAD is still the user's second commit, not a third
	// auto-injected one.
	out, err := exec.Command("git", "-C", dir, "log", "--format=%s").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		t.Errorf("commit count = %d, want 2 (no extra commit injected)", len(lines))
	}
	if lines[0] != "real-second-commit" {
		t.Errorf("HEAD subject = %q, want 'real-second-commit'", lines[0])
	}
}

// captureHEAD returns the HEAD SHA in dir. Helper for the
// idempotency test.
func captureHEAD(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// mustGit runs a git command in dir and fails the test on error.
// Distinct from session_test.go's runGit only in name (Go's
// per-package symbol scope) — kept inline so this file is self-
// contained.
func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
