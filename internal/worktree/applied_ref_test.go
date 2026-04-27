package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppliedRef_Stable locks the ref-name shape since /merge,
// recovery instructions, and any operator following docs all rely
// on the same string format.
func TestAppliedRef_Stable(t *testing.T) {
	got := AppliedRef("plan-1234-5678")
	want := "refs/codrax/applied/plan-1234-5678"
	if got != want {
		t.Errorf("AppliedRef = %q, want %q", got, want)
	}
}

// TestTagAppliedCommit_PinsRef verifies that TagAppliedCommit creates
// a ref pointing at the supplied SHA in the main repo, and that the
// ref survives `worktree remove` (the whole point of this surface).
func TestTagAppliedCommit_PinsRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mainRepo := initBareRepo(t)
	// Make a commit so HEAD is real.
	writeAndCommit(t, mainRepo, "seed.txt", "seed\n", "seed commit")

	// Create a worktree, add a commit there.
	base := t.TempDir()
	sess, err := Create(base, mainRepo, "tag-test-trace")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeAndCommit(t, sess.Path(), "applied.txt", "apply\n", "codrax apply iter (plan=plan-test-001)")
	sha := strings.TrimSpace(runOrFatal(t, sess.Path(), "rev-parse", "HEAD"))

	ref, err := TagAppliedCommit(mainRepo, "plan-test-001", sha)
	if err != nil {
		t.Fatalf("TagAppliedCommit: %v", err)
	}
	if ref != "refs/codrax/applied/plan-test-001" {
		t.Errorf("ref name = %q", ref)
	}

	// Discard the worktree — the recovery ref MUST still resolve in
	// the main repo afterward (this is the whole point of pinning).
	if err := sess.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	resolved := strings.TrimSpace(runOrFatal(t, mainRepo, "rev-parse", ref))
	if resolved != sha {
		t.Errorf("after Discard: ref %s resolves to %q, want %q", ref, resolved, sha)
	}

	// DeleteAppliedRef cleans up.
	if err := DeleteAppliedRef(mainRepo, "plan-test-001"); err != nil {
		t.Fatalf("DeleteAppliedRef: %v", err)
	}
	cmd := exec.Command("git", "rev-parse", "--verify", "--quiet", ref)
	cmd.Dir = mainRepo
	if cmd.Run() == nil {
		t.Errorf("ref %s should be gone after DeleteAppliedRef", ref)
	}
}

// TestIsWorkingTreeDirty_IgnoresUntracked locks the customer-
// reported behaviour: untracked files (codrax's own .codrax/
// runtime dir, the user's .venv/) MUST NOT block /merge. Pre-fix
// the dirty check used `git status --porcelain` (no flag) and
// blocked on `?? .codrax/` lines; post-fix it uses
// `--untracked-files=no` so only modified/staged TRACKED files
// count as dirty.
func TestIsWorkingTreeDirty_IgnoresUntracked(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	repo := initBareRepo(t)
	writeAndCommit(t, repo, "tracked.txt", "v1\n", "seed")

	// Untracked files reproducing the customer's `?? .codrax/`
	// + `?? .venv/` shape should NOT mark the tree dirty.
	for _, p := range []string{".codrax/plans/x.json", ".venv/bin/python"} {
		full := filepath.Join(repo, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte("stub"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	dirty, err := isWorkingTreeDirty(repo)
	if err != nil {
		t.Fatalf("isWorkingTreeDirty: %v", err)
	}
	if dirty {
		t.Errorf("untracked files alone should NOT mark tree dirty")
	}

	// Modify a TRACKED file → must mark dirty.
	if err := os.WriteFile(filepath.Join(repo, "tracked.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("modify tracked: %v", err)
	}
	dirty, err = isWorkingTreeDirty(repo)
	if err != nil {
		t.Fatalf("isWorkingTreeDirty after modify: %v", err)
	}
	if !dirty {
		t.Error("tracked-file modification should mark tree dirty")
	}
}

// TestMergeFromRef_FastForward drives the keep_on_success=false
// recovery path: tag a commit, discard the worktree, then fold the
// ref into the main branch.
func TestMergeFromRef_FastForward(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mainRepo := initBareRepo(t)
	writeAndCommit(t, mainRepo, "seed.txt", "seed\n", "seed commit")
	branch := strings.TrimSpace(runOrFatal(t, mainRepo, "rev-parse", "--abbrev-ref", "HEAD"))

	base := t.TempDir()
	sess, err := Create(base, mainRepo, "merge-from-ref-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeAndCommit(t, sess.Path(), "guess.py", "print('hi')\n", "codrax apply iter (plan=plan-merge-001)")
	sha := strings.TrimSpace(runOrFatal(t, sess.Path(), "rev-parse", "HEAD"))

	if _, err := TagAppliedCommit(mainRepo, "plan-merge-001", sha); err != nil {
		t.Fatalf("TagAppliedCommit: %v", err)
	}
	if err := sess.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	// After Discard, run MergeFromRef. The current branch should
	// fast-forward to the apply commit.
	res, err := MergeFromRef(MergeFromRefOptions{
		MainRepoRoot: mainRepo,
		Ref:          AppliedRef("plan-merge-001"),
		BaseBranch:   branch,
		TargetBranch: branch,
		Mode:         MergeAuto,
	})
	if err != nil {
		t.Fatalf("MergeFromRef: %v", err)
	}
	if res.Strategy != "fast_forward" {
		t.Errorf("strategy = %q, want fast_forward", res.Strategy)
	}
	if res.FinalBranch != branch {
		t.Errorf("finalBranch = %q, want %q", res.FinalBranch, branch)
	}
	// Verify the file landed.
	checkFile(t, mainRepo, "guess.py", "print('hi')\n")
}

// initBareRepo creates a fresh git repo for use as the main repo in
// the tests above. Identity is set so commits don't fail on bare
// fixture machines.
func initBareRepo(t *testing.T) string {
	t.Helper()
	clearActiveSessions(t)
	dir := t.TempDir()
	mustRunGit(t, dir, "init", "-q")
	mustRunGit(t, dir, "config", "user.email", "test@codrax")
	mustRunGit(t, dir, "config", "user.name", "test-user")
	mustRunGit(t, dir, "config", "core.autocrlf", "false")
	mustRunGit(t, dir, "config", "core.eol", "lf")
	return dir
}

func writeAndCommit(t *testing.T, repo, path, content, msg string) {
	t.Helper()
	full := filepath.Join(repo, path)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	mustRunGit(t, repo, "add", path)
	mustRunGit(t, repo, "commit", "-q", "-m", msg)
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v in %s: %v (%s)", args, dir, err, out)
	}
}

func runOrFatal(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v (%s)", args, dir, err, out)
	}
	return string(out)
}

func checkFile(t *testing.T, repo, path, want string) {
	t.Helper()
	full := filepath.Join(repo, path)
	got, err := os.ReadFile(full)
	if err != nil {
		t.Fatalf("read %s: %v", full, err)
	}
	if string(got) != want {
		t.Errorf("%s contents = %q, want %q", path, string(got), want)
	}
}
