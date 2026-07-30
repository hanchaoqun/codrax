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

// TestMergeFromRef_NewBranchLandsWholeChain pins SWEEPFIX S11 (the #8
// fix shipped unpinned): the MergeNewBranch ref-lane must cherry-pick
// EVERY commit ahead of base oldest-first — a chain where the landed
// units sit in PARENTS of the tip (the W-3 partial-checkpoint shape)
// loses its earlier commits if only the tip is picked.
func TestMergeFromRef_NewBranchLandsWholeChain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mainRepo := initBareRepo(t)
	writeAndCommit(t, mainRepo, "seed.txt", "seed\n", "seed commit")
	branch := strings.TrimSpace(runOrFatal(t, mainRepo, "rev-parse", "--abbrev-ref", "HEAD"))

	base := t.TempDir()
	sess, err := Create(base, mainRepo, "merge-chain-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeAndCommit(t, sess.Path(), "unit1.txt", "first unit\n", "codrax apply iter 1 (plan=plan-chain-001)")
	writeAndCommit(t, sess.Path(), "unit2.txt", "second unit\n", "codrax apply iter 2 (plan=plan-chain-001)")
	tip := strings.TrimSpace(runOrFatal(t, sess.Path(), "rev-parse", "HEAD"))
	if _, err := TagAppliedCommit(mainRepo, "plan-chain-001", tip); err != nil {
		t.Fatalf("TagAppliedCommit: %v", err)
	}
	if err := sess.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	// Diverge base so fast-forward is impossible and the cherry-pick
	// branch lane engages.
	writeAndCommit(t, mainRepo, "diverge.txt", "base moved on\n", "unrelated base commit")

	res, err := MergeFromRef(MergeFromRefOptions{
		MainRepoRoot: mainRepo,
		Ref:          AppliedRef("plan-chain-001"),
		BaseBranch:   branch,
		TargetBranch: "codrax/landing-chain",
		Mode:         MergeNewBranch,
	})
	if err != nil {
		t.Fatalf("MergeFromRef: %v", err)
	}
	if res.Strategy != "cherry_pick_branch" {
		t.Errorf("strategy = %q, want cherry_pick_branch", res.Strategy)
	}
	if len(res.CommitsLanded) != 2 {
		t.Errorf("CommitsLanded = %d commits, want the whole 2-commit chain", len(res.CommitsLanded))
	}
	runOrFatal(t, mainRepo, "checkout", "codrax/landing-chain")
	for _, want := range []string{"unit1.txt", "unit2.txt"} {
		if _, err := os.Stat(filepath.Join(mainRepo, want)); err != nil {
			t.Errorf("%s missing on target branch — earlier chain commit dropped: %v", want, err)
		}
	}
}

// TestMergeFromRef_SkipsAlreadyAppliedCommit pins the S7/S8 root fix
// as refined by round-3 R4/R5: an already-applied mid-chain commit is
// skipped via PICK-TIME structural classification (CHERRY_PICK_HEAD
// present + porcelain-clean tree -> `cherry-pick --quit`) — no
// output-phrase parsing (which mis-skipped genuine conflicts whose
// free-text subject contained "nothing to commit" and never matched
// under localized git), and no patch-id precompute (which silently
// dropped a re-apply after base had REVERTED the commit).
func TestMergeFromRef_SkipsAlreadyAppliedCommit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mainRepo := initBareRepo(t)
	writeAndCommit(t, mainRepo, "seed.txt", "seed\n", "seed commit")
	branch := strings.TrimSpace(runOrFatal(t, mainRepo, "rev-parse", "--abbrev-ref", "HEAD"))

	base := t.TempDir()
	sess, err := Create(base, mainRepo, "merge-equiv-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Chain commit 1: content that will ALSO land on base (patch-equal).
	writeAndCommit(t, sess.Path(), "shared.txt", "identical change\n", "codrax apply iter 1 (plan=plan-eq-001)")
	writeAndCommit(t, sess.Path(), "extra.txt", "only in chain\n", "codrax apply iter 2 (plan=plan-eq-001)")
	tip := strings.TrimSpace(runOrFatal(t, sess.Path(), "rev-parse", "HEAD"))
	if _, err := TagAppliedCommit(mainRepo, "plan-eq-001", tip); err != nil {
		t.Fatalf("TagAppliedCommit: %v", err)
	}
	if err := sess.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	// Base independently gains the SAME patch (different author time →
	// different SHA, same patch-id) plus a divergence commit.
	writeAndCommit(t, mainRepo, "shared.txt", "identical change\n", "same change landed independently")
	writeAndCommit(t, mainRepo, "diverge.txt", "base moved on\n", "unrelated base commit")

	if _, err := MergeFromRef(MergeFromRefOptions{
		MainRepoRoot: mainRepo,
		Ref:          AppliedRef("plan-eq-001"),
		BaseBranch:   branch,
		TargetBranch: "codrax/landing-eq",
		Mode:         MergeNewBranch,
	}); err != nil {
		t.Fatalf("MergeFromRef must skip the patch-equivalent commit, not conflict: %v", err)
	}
	runOrFatal(t, mainRepo, "checkout", "codrax/landing-eq")
	if _, err := os.Stat(filepath.Join(mainRepo, "extra.txt")); err != nil {
		t.Errorf("non-equivalent chain commit must land: %v", err)
	}
}

// TestMergeFromRef_RevertedOnBaseLandsAgain pins round-3 R4 (high): a
// chain commit whose patch base once applied and then REVERTED must
// land again — the patch-id precompute this replaced marked it
// already-applied and silently dropped the user's change while /merge
// reported success (the S7 silent-drop class, reintroduced one lane
// over).
func TestMergeFromRef_RevertedOnBaseLandsAgain(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mainRepo := initBareRepo(t)
	writeAndCommit(t, mainRepo, "seed.txt", "seed\n", "seed commit")
	branch := strings.TrimSpace(runOrFatal(t, mainRepo, "rev-parse", "--abbrev-ref", "HEAD"))

	base := t.TempDir()
	sess, err := Create(base, mainRepo, "merge-revert-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeAndCommit(t, sess.Path(), "feature.txt", "the change\n", "codrax apply iter 1 (plan=plan-rv-001)")
	tip := strings.TrimSpace(runOrFatal(t, sess.Path(), "rev-parse", "HEAD"))
	if _, err := TagAppliedCommit(mainRepo, "plan-rv-001", tip); err != nil {
		t.Fatalf("TagAppliedCommit: %v", err)
	}
	if err := sess.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	// Base independently lands the same patch, then REVERTS it, then
	// diverges. Patch-id equivalence still sees the applied copy.
	writeAndCommit(t, mainRepo, "feature.txt", "the change\n", "same change landed independently")
	runOrFatal(t, mainRepo, "revert", "--no-edit", "HEAD")
	writeAndCommit(t, mainRepo, "diverge.txt", "base moved on\n", "unrelated base commit")

	res, err := MergeFromRef(MergeFromRefOptions{
		MainRepoRoot: mainRepo,
		Ref:          AppliedRef("plan-rv-001"),
		BaseBranch:   branch,
		TargetBranch: "codrax/landing-rv",
		Mode:         MergeNewBranch,
	})
	if err != nil {
		t.Fatalf("MergeFromRef: %v", err)
	}
	if len(res.CommitsLanded) != 1 {
		t.Fatalf("the reverted change must LAND again, not be skipped; landed=%v", res.CommitsLanded)
	}
	runOrFatal(t, mainRepo, "checkout", "codrax/landing-rv")
	got, err := os.ReadFile(filepath.Join(mainRepo, "feature.txt"))
	if err != nil || string(got) != "the change\n" {
		t.Fatalf("re-applied content missing on target: %q %v", got, err)
	}
}

// TestMergeFromRef_SubjectSayingNothingToCommitStillConflicts pins the
// S7 regression shape directly: a commit whose SUBJECT contains
// "nothing to commit" but whose pick genuinely conflicts must ABORT
// loudly — never be skipped as already-applied.
func TestMergeFromRef_SubjectSayingNothingToCommitStillConflicts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	mainRepo := initBareRepo(t)
	writeAndCommit(t, mainRepo, "file.txt", "line1\n", "seed commit")
	branch := strings.TrimSpace(runOrFatal(t, mainRepo, "rev-parse", "--abbrev-ref", "HEAD"))

	base := t.TempDir()
	sess, err := Create(base, mainRepo, "merge-conflict-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	writeAndCommit(t, sess.Path(), "file.txt", "chain version\n", "fix: nothing to commit twice bug (plan=plan-cf-001)")
	tip := strings.TrimSpace(runOrFatal(t, sess.Path(), "rev-parse", "HEAD"))
	if _, err := TagAppliedCommit(mainRepo, "plan-cf-001", tip); err != nil {
		t.Fatalf("TagAppliedCommit: %v", err)
	}
	if err := sess.Discard(); err != nil {
		t.Fatalf("Discard: %v", err)
	}
	// Base rewrites the same line differently → genuine conflict.
	writeAndCommit(t, mainRepo, "file.txt", "base version\n", "conflicting base change")

	_, err = MergeFromRef(MergeFromRefOptions{
		MainRepoRoot: mainRepo,
		Ref:          AppliedRef("plan-cf-001"),
		BaseBranch:   branch,
		TargetBranch: "codrax/landing-cf",
		Mode:         MergeNewBranch,
	})
	if err == nil {
		t.Fatal("a genuine conflict must abort — silently skipping drops the user's change")
	}
	if branchExists(mainRepo, "codrax/landing-cf") {
		t.Error("rollback must delete the target branch")
	}
	if got := strings.TrimSpace(runOrFatal(t, mainRepo, "rev-parse", "--abbrev-ref", "HEAD")); got != branch {
		t.Errorf("rollback must restore prior branch; on %q want %q", got, branch)
	}
}
