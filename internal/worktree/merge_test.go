package worktree

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupMergeFixture builds a main repo with one base commit, a
// detached worktree forked from it, and adds extra commits inside
// the worktree so MergeIntoBranch has something to fold back. The
// test gets a self-contained fixture with pinned SHAs so failure
// modes (conflict, non-ff, dirty tree) can be exercised
// deterministically.
func setupMergeFixture(t *testing.T) (mainRoot, wtPath, baseBranch string) {
	t.Helper()

	mainRoot = t.TempDir()
	wtBase := t.TempDir()

	// Main repo with one commit on `main`.
	mustGit(t, mainRoot, "init", "--initial-branch=main")
	mustGit(t, mainRoot, "config", "user.email", "test@test")
	mustGit(t, mainRoot, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(mainRoot, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	mustGit(t, mainRoot, "add", "README.md")
	mustGit(t, mainRoot, "commit", "-m", "base")

	// Detached worktree fork.
	sess, err := Create(wtBase, mainRoot, "trace-merge-test")
	if err != nil {
		t.Fatalf("worktree.Create: %v", err)
	}
	wtPath = sess.Path()
	t.Cleanup(func() {
		// Discard the worktree; the merge tests sometimes leave
		// branches behind on mainRoot, but t.TempDir handles that.
		_ = sess.Discard()
	})

	// Two new commits in the worktree.
	mustGit(t, wtPath, "config", "user.email", "test@test")
	mustGit(t, wtPath, "config", "user.name", "test")
	if err := os.WriteFile(filepath.Join(wtPath, "feature.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	mustGit(t, wtPath, "add", "feature.txt")
	mustGit(t, wtPath, "commit", "-m", "feat: add feature")
	if err := os.WriteFile(filepath.Join(wtPath, "feature.txt"), []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	mustGit(t, wtPath, "add", "feature.txt")
	mustGit(t, wtPath, "commit", "-m", "feat: extend feature")

	return mainRoot, wtPath, "main"
}

// TestMergeIntoBranch_FastForward verifies the simplest happy path:
// the worktree is strictly ahead of base, target branch == base,
// MergeAuto picks fast_forward. main moves forward; the worktree
// commits land on `main` byte-for-byte.
func TestMergeIntoBranch_FastForward(t *testing.T) {
	main, wt, base := setupMergeFixture(t)
	preBase, _ := runGitCapture(main, "rev-parse", base)
	preWT, _ := runGitCapture(wt, "rev-parse", "HEAD")

	res, err := MergeIntoBranch(MergeOptions{
		MainRepoRoot: main,
		WorktreePath: wt,
		BaseBranch:   base,
		TargetBranch: base, // same → ff path
		Mode:         MergeAuto,
	})
	if err != nil {
		t.Fatalf("MergeIntoBranch: %v", err)
	}
	if res.Strategy != "fast_forward" {
		t.Errorf("strategy = %q, want fast_forward", res.Strategy)
	}
	if len(res.CommitsLanded) != 2 {
		t.Errorf("CommitsLanded = %v, want 2 commits", res.CommitsLanded)
	}
	if res.FinalBranch != base {
		t.Errorf("FinalBranch = %q, want %q", res.FinalBranch, base)
	}

	postBase, _ := runGitCapture(main, "rev-parse", base)
	if strings.TrimSpace(preBase) == strings.TrimSpace(postBase) {
		t.Error("base branch should have moved")
	}
	if strings.TrimSpace(postBase) != strings.TrimSpace(preWT) {
		t.Errorf("base = %q, worktree HEAD = %q — expected ff to align them",
			postBase, preWT)
	}
}

// TestMergeIntoBranch_NewBranch verifies the PR workflow: target
// branch differs from base, new branch is created, commits cherry-
// picked, base stays untouched.
func TestMergeIntoBranch_NewBranch(t *testing.T) {
	main, wt, base := setupMergeFixture(t)
	preBase, _ := runGitCapture(main, "rev-parse", base)

	res, err := MergeIntoBranch(MergeOptions{
		MainRepoRoot: main,
		WorktreePath: wt,
		BaseBranch:   base,
		TargetBranch: "fix/codrax-1",
		Mode:         MergeAuto,
	})
	if err != nil {
		t.Fatalf("MergeIntoBranch: %v", err)
	}
	if res.Strategy != "cherry_pick_branch" {
		t.Errorf("strategy = %q, want cherry_pick_branch", res.Strategy)
	}
	if res.FinalBranch != "fix/codrax-1" {
		t.Errorf("FinalBranch = %q, want fix/codrax-1", res.FinalBranch)
	}

	// Base must stay where it was.
	postBase, _ := runGitCapture(main, "rev-parse", base)
	if strings.TrimSpace(preBase) != strings.TrimSpace(postBase) {
		t.Errorf("base moved when it should not have: pre=%s post=%s", preBase, postBase)
	}

	// New branch must exist and contain the cherry-picked commits.
	newBranchSHA, _ := runGitCapture(main, "rev-parse", "fix/codrax-1")
	if strings.TrimSpace(newBranchSHA) == "" {
		t.Error("fix/codrax-1 branch missing")
	}
	count, _ := runGitCapture(main, "rev-list", "--count", base+"..fix/codrax-1")
	if strings.TrimSpace(count) != "2" {
		t.Errorf("commits on fix/codrax-1 vs %s = %q, want 2", base, count)
	}
}

// TestMergeIntoBranch_NothingToMerge handles the no-op case: a
// freshly created worktree that was never touched. Helper returns
// ErrNothingToMerge so the REPL can surface the right message.
func TestMergeIntoBranch_NothingToMerge(t *testing.T) {
	mainRoot := t.TempDir()
	wtBase := t.TempDir()
	mustGit(t, mainRoot, "init", "--initial-branch=main")
	mustGit(t, mainRoot, "config", "user.email", "test@test")
	mustGit(t, mainRoot, "config", "user.name", "test")
	mustGit(t, mainRoot, "commit", "--allow-empty", "-m", "base")
	sess, err := Create(wtBase, mainRoot, "trace-noop")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = sess.Discard() })

	_, err = MergeIntoBranch(MergeOptions{
		MainRepoRoot: mainRoot,
		WorktreePath: sess.Path(),
		BaseBranch:   "main",
		TargetBranch: "main",
		Mode:         MergeAuto,
	})
	if !errors.Is(err, ErrNothingToMerge) {
		t.Errorf("err = %v, want ErrNothingToMerge", err)
	}
}

// TestMergeIntoBranch_TargetExistsRefuses verifies new-branch mode
// refuses to silently rewrite an existing branch.
func TestMergeIntoBranch_TargetExistsRefuses(t *testing.T) {
	main, wt, base := setupMergeFixture(t)
	mustGit(t, main, "branch", "fix/already-exists")

	_, err := MergeIntoBranch(MergeOptions{
		MainRepoRoot: main,
		WorktreePath: wt,
		BaseBranch:   base,
		TargetBranch: "fix/already-exists",
		Mode:         MergeNewBranch,
	})
	if !errors.Is(err, ErrTargetBranchExists) {
		t.Errorf("err = %v, want ErrTargetBranchExists", err)
	}
}

// TestMergeIntoBranch_FFOnlyRejectsNonFF verifies the ff-only path
// returns ErrNonFastForward when the base has diverged. Setup
// commits something on main that's not in the worktree so an ff
// is impossible.
func TestMergeIntoBranch_FFOnlyRejectsNonFF(t *testing.T) {
	main, wt, base := setupMergeFixture(t)
	// Diverge: add a commit on main that the worktree didn't see.
	if err := os.WriteFile(filepath.Join(main, "diverge.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, main, "add", "diverge.txt")
	mustGit(t, main, "commit", "-m", "diverge")

	_, err := MergeIntoBranch(MergeOptions{
		MainRepoRoot: main,
		WorktreePath: wt,
		BaseBranch:   base,
		TargetBranch: base,
		Mode:         MergeFastForwardOnly,
	})
	if !errors.Is(err, ErrNonFastForward) {
		t.Errorf("err = %v, want ErrNonFastForward", err)
	}
}

// TestMergeIntoBranch_DirtyWorkingTreeRefused verifies the helper
// refuses to merge when the main checkout has uncommitted local
// changes — silently fast-forwarding past them would lose user work.
func TestMergeIntoBranch_DirtyWorkingTreeRefused(t *testing.T) {
	main, wt, base := setupMergeFixture(t)
	if err := os.WriteFile(filepath.Join(main, "dirty.txt"), []byte("local"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, main, "add", "dirty.txt")
	// Not committed → working tree dirty (staged).

	_, err := MergeIntoBranch(MergeOptions{
		MainRepoRoot: main,
		WorktreePath: wt,
		BaseBranch:   base,
		TargetBranch: base,
		Mode:         MergeAuto,
	})
	if err == nil {
		t.Fatal("expected error on dirty working tree")
	}
	if !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("error should mention 'uncommitted'; got: %v", err)
	}
}

// TestMergeIntoBranch_CherryPickConflictRollsBack verifies that a
// conflict during cherry-pick aborts cleanly: the half-created
// branch is deleted, main repo lands back on prior branch, no
// uncommitted state lingers.
func TestMergeIntoBranch_CherryPickConflictRollsBack(t *testing.T) {
	main, wt, base := setupMergeFixture(t)
	// Make `main` and the worktree edit the SAME line to provoke a
	// cherry-pick conflict. Worktree already has feature.txt; add
	// a colliding feature.txt to main.
	if err := os.WriteFile(filepath.Join(main, "feature.txt"), []byte("MAIN_VERSION\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	mustGit(t, main, "add", "feature.txt")
	mustGit(t, main, "commit", "-m", "main edits feature.txt independently")

	priorHEAD, _ := runGitCapture(main, "rev-parse", "HEAD")

	_, err := MergeIntoBranch(MergeOptions{
		MainRepoRoot: main,
		WorktreePath: wt,
		BaseBranch:   base,
		TargetBranch: "fix/will-conflict",
		Mode:         MergeNewBranch,
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "conflicted") {
		t.Errorf("error should mention 'conflicted'; got: %v", err)
	}
	// Throwaway branch should be gone.
	if exec.Command("git", "-C", main, "rev-parse", "--verify", "refs/heads/fix/will-conflict").Run() == nil {
		t.Error("conflict cleanup failed: fix/will-conflict still exists")
	}
	// Main HEAD should be back where it was.
	postHEAD, _ := runGitCapture(main, "rev-parse", "HEAD")
	if strings.TrimSpace(priorHEAD) != strings.TrimSpace(postHEAD) {
		t.Errorf("HEAD should restore: prior=%s post=%s", priorHEAD, postHEAD)
	}
	// Working tree should be clean.
	dirty, _ := runGitCapture(main, "status", "--porcelain")
	if strings.TrimSpace(dirty) != "" {
		t.Errorf("working tree should be clean post-rollback; got: %q", dirty)
	}
}

// TestMergeIntoBranch_ForceNewBranchOverFF verifies MergeNewBranch
// always creates a branch even when fast-forward would have been
// possible. This is the "I want a PR" workflow.
func TestMergeIntoBranch_ForceNewBranchOverFF(t *testing.T) {
	main, wt, base := setupMergeFixture(t)
	preBase, _ := runGitCapture(main, "rev-parse", base)

	res, err := MergeIntoBranch(MergeOptions{
		MainRepoRoot: main,
		WorktreePath: wt,
		BaseBranch:   base,
		TargetBranch: "fix/force-branch",
		Mode:         MergeNewBranch,
	})
	if err != nil {
		t.Fatalf("MergeIntoBranch: %v", err)
	}
	if res.Strategy != "cherry_pick_branch" {
		t.Errorf("strategy = %q, want cherry_pick_branch", res.Strategy)
	}
	postBase, _ := runGitCapture(main, "rev-parse", base)
	if strings.TrimSpace(preBase) != strings.TrimSpace(postBase) {
		t.Errorf("base moved on forced new-branch path: pre=%s post=%s", preBase, postBase)
	}
}
