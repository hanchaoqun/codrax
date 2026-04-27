package repl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitBranchInit creates a fresh git repo with one commit on a
// named branch, returns the absolute path. Test helper used by
// the git-branch and merge-target tests so each case starts from
// a clean known state.
func gitBranchInit(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	mustGitOK := func(args ...string) {
		c := exec.Command("git", append([]string{"-C", dir}, args...)...)
		c.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test",
		)
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	mustGitOK("init", "--initial-branch="+branch)
	mustGitOK("config", "user.email", "test@test")
	mustGitOK("config", "user.name", "test")
	mustGitOK("commit", "--allow-empty", "-m", "init")
	abs, _ := filepath.Abs(dir)
	return abs
}

// TestGitBranchProbe_Branch covers the happy path: HEAD is on a
// branch, gitBranchProbe returns the branch name.
func TestGitBranchProbe_Branch(t *testing.T) {
	repo := gitBranchInit(t, "main")
	if got := gitBranchProbe(repo); got != "main" {
		t.Errorf("gitBranchProbe = %q, want main", got)
	}

	// Switch to a feature branch and re-probe.
	cmd := exec.Command("git", "-C", repo, "checkout", "-b", "feature-x")
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout -b: %v\n%s", err, out)
	}
	if got := gitBranchProbe(repo); got != "feature-x" {
		t.Errorf("after checkout -b feature-x: probe = %q, want feature-x", got)
	}
}

// TestGitBranchProbe_DetachedHead covers the rebase / cherry-pick
// / explicit-SHA-checkout case where HEAD is detached. probe
// should return "detached@<sha7>", never empty.
func TestGitBranchProbe_DetachedHead(t *testing.T) {
	repo := gitBranchInit(t, "main")
	// Resolve HEAD's SHA.
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	sha := strings.TrimSpace(string(out))
	// Detach.
	if err := exec.Command("git", "-C", repo, "checkout", "--detach", sha).Run(); err != nil {
		t.Fatalf("detach: %v", err)
	}
	got := gitBranchProbe(repo)
	if !strings.HasPrefix(got, "detached@") {
		t.Errorf("detached probe = %q, want detached@<sha>", got)
	}
	if !strings.Contains(got, sha[:7]) {
		t.Errorf("detached probe = %q, want it to contain SHA prefix %q", got, sha[:7])
	}
}

// TestGitBranchProbe_NotARepo verifies the helper returns empty
// (NOT errors, NOT panics) when the path isn't a git repo. The
// caller drops the [git:*] marker entirely in that case.
func TestGitBranchProbe_NotARepo(t *testing.T) {
	bare := t.TempDir()
	if got := gitBranchProbe(bare); got != "" {
		t.Errorf("gitBranchProbe on non-repo = %q, want empty", got)
	}
}

// TestGitBranchProbe_EmptyPath defends against accidental empty
// repoRoot — should return empty without invoking git.
func TestGitBranchProbe_EmptyPath(t *testing.T) {
	if got := gitBranchProbe(""); got != "" {
		t.Errorf("gitBranchProbe(\"\") = %q, want empty", got)
	}
	if got := gitBranchProbe("   "); got != "" {
		t.Errorf("gitBranchProbe(spaces) = %q, want empty", got)
	}
}

// TestDefaultMergeTarget_LiveBranchWins covers the new contract:
// /merge default = live git HEAD, not the configured --branch
// flag value. r.branch is the fallback only.
func TestDefaultMergeTarget_LiveBranchWins(t *testing.T) {
	repo := gitBranchInit(t, "feature-x")
	// configured says "main" (e.g. user passed --branch=main at
	// startup), but the live HEAD is on feature-x. Default merge
	// target must follow the live HEAD.
	got := defaultMergeTarget(repo, "main")
	if got != "feature-x" {
		t.Errorf("defaultMergeTarget live=feature-x configured=main = %q, want feature-x", got)
	}
}

// TestDefaultMergeTarget_DetachedFallsBackToConfigured verifies
// the fallback: detached HEAD isn't a meaningful merge target so
// the configured value (--branch flag) is used instead.
func TestDefaultMergeTarget_DetachedFallsBackToConfigured(t *testing.T) {
	repo := gitBranchInit(t, "main")
	out, _ := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	sha := strings.TrimSpace(string(out))
	_ = exec.Command("git", "-C", repo, "checkout", "--detach", sha).Run()

	got := defaultMergeTarget(repo, "main")
	if got != "main" {
		t.Errorf("defaultMergeTarget on detached HEAD = %q, want fallback to configured 'main'", got)
	}
}

// TestDefaultMergeTarget_NotARepoFallsBackToConfigured: outside
// a git repo, also fall back to configured.
func TestDefaultMergeTarget_NotARepoFallsBackToConfigured(t *testing.T) {
	bare := t.TempDir()
	got := defaultMergeTarget(bare, "main")
	if got != "main" {
		t.Errorf("defaultMergeTarget non-repo = %q, want fallback 'main'", got)
	}
}
