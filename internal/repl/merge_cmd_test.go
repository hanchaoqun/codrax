package repl

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestParseMergeToArg covers the flag parser for `/approve
// --merge-to=<branch>` (and the bare `--merge-to <branch>` shape).
// Empty / unknown flags return "".
func TestParseMergeToArg(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"/approve", ""},
		{"/approve --merge-to=main", "main"},
		{"/approve --merge-to=fix/foo", "fix/foo"},
		{"/approve  --merge-to=fix/foo  ", "fix/foo"},
		{"/approve --merge-to fix/foo", "fix/foo"},
		{"/approve --auto-init --merge-to=fix/x", "fix/x"},
		{"/approve --unknown=xxx", ""},
	} {
		got := parseMergeToArg(tc.in)
		if got != tc.want {
			t.Errorf("parseMergeToArg(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// setupMergeReplFixture builds a fully wired REPL:
//   - main repo with one base commit
//   - second checkout "worktree" with two extra commits (simulating a
//     successful /approve aftermath)
//   - PlanStore with one Status=applied plan whose WorktreePath
//     points at the worktree
//
// Returns the REPL, plan id, worktree path, and main repo root.
// Tests can drive /merge end-to-end against a real git tree.
func setupMergeReplFixture(t *testing.T) (r *REPL, planID, wt, main string) {
	t.Helper()
	main = t.TempDir()
	wtDir := t.TempDir()

	// Build main repo with one base commit so HEAD resolves.
	gitInDir(t, main, "init", "--initial-branch=main")
	gitInDir(t, main, "config", "user.email", "x@y")
	gitInDir(t, main, "config", "user.name", "x")
	if err := os.WriteFile(filepath.Join(main, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	gitInDir(t, main, "add", "README.md")
	gitInDir(t, main, "commit", "-m", "base")

	// Set up worktree as a detached checkout with new commits.
	wt = filepath.Join(wtDir, "worktree-checkout")
	gitInDir(t, main, "worktree", "add", "--detach", wt, "HEAD")
	gitInDir(t, wt, "config", "user.email", "x@y")
	gitInDir(t, wt, "config", "user.name", "x")
	if err := os.WriteFile(filepath.Join(wt, "feature.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	gitInDir(t, wt, "add", "feature.txt")
	gitInDir(t, wt, "commit", "-m", "feat: hi")

	// PlanStore with one applied plan pointing at the worktree.
	store := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:           "plan-merge-1",
		Summary:      "merge fixture",
		Status:       types.PlanStatusApplied,
		WorktreePath: wt,
		Changes:      []types.FileChange{{Path: "feature.txt", Kind: "create"}},
		TargetPaths:  []string{"feature.txt"},
	}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("PlanStore.Save: %v", err)
	}

	r, _ = newScriptedREPL(t, store)
	r.repoRoot = main
	r.branch = "main"

	return r, plan.ID, wt, main
}

// TestHandleMergeCmd_FastForwardSuccess verifies the happy path:
// /merge picks the most recent applied plan, fast-forwards into
// r.branch, prints the success message, and discards the worktree.
func TestHandleMergeCmd_FastForwardSuccess(t *testing.T) {
	r, _, wt, main := setupMergeReplFixture(t)
	preBase, _ := exec.Command("git", "-C", main, "rev-parse", "main").Output()
	preWT, _ := exec.Command("git", "-C", wt, "rev-parse", "HEAD").Output()

	// Bypass the interactive confirm so the test is deterministic.
	r.in = nil // sets interactive() = true; but we want non-interactive.
	r.in = strings.NewReader("")

	out, ok := captureOut(r)
	if !ok {
		t.Fatal("could not capture out buffer")
	}

	r.handleMergeCmd("/merge")

	got := out.String()
	if !strings.Contains(got, "Fast-forwarded") {
		t.Errorf("expected 'Fast-forwarded' in output; got: %q", got)
	}

	postBase, _ := exec.Command("git", "-C", main, "rev-parse", "main").Output()
	if strings.TrimSpace(string(preBase)) == strings.TrimSpace(string(postBase)) {
		t.Error("main branch did not move after fast-forward")
	}
	if strings.TrimSpace(string(postBase)) != strings.TrimSpace(string(preWT)) {
		t.Errorf("main = %s, worktree HEAD = %s — expected ff to align them",
			postBase, preWT)
	}

	// Worktree should be discarded post-merge.
	if _, err := os.Stat(wt); err == nil {
		t.Error("worktree dir should have been removed after merge")
	}
}

// TestHandleMergeCmd_NewBranchSuccess verifies --branch=<name>
// triggers cherry_pick_branch, leaves main untouched, and lands the
// commit on the new branch.
func TestHandleMergeCmd_NewBranchSuccess(t *testing.T) {
	r, _, _, main := setupMergeReplFixture(t)
	preMain, _ := exec.Command("git", "-C", main, "rev-parse", "main").Output()
	r.in = strings.NewReader("")

	out, _ := captureOut(r)
	r.handleMergeCmd("/merge --branch=fix/codrax-1")

	got := out.String()
	if !strings.Contains(got, "fix/codrax-1") {
		t.Errorf("expected new branch name in output; got: %q", got)
	}
	postMain, _ := exec.Command("git", "-C", main, "rev-parse", "main").Output()
	if strings.TrimSpace(string(preMain)) != strings.TrimSpace(string(postMain)) {
		t.Errorf("main moved on new-branch path: pre=%s post=%s", preMain, postMain)
	}
	branchSHA, err := exec.Command("git", "-C", main, "rev-parse", "fix/codrax-1").Output()
	if err != nil || strings.TrimSpace(string(branchSHA)) == "" {
		t.Errorf("fix/codrax-1 branch missing or unresolvable: %v", err)
	}
}

// TestHandleMergeCmd_NoApplyYet verifies the error path when no
// applied plan with a preserved worktree exists.
func TestHandleMergeCmd_NoApplyYet(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, _ := newScriptedREPL(t, store)
	out, _ := captureOut(r)

	r.handleMergeCmd("/merge")

	got := out.String()
	if !strings.Contains(strings.ToLower(got), "no worktree to merge from") {
		t.Errorf("expected 'no worktree to merge from'; got: %q", got)
	}
}

// TestHandleMergeCmd_WriteDisabled verifies the L2 gate fires when
// codrax.yaml has write_enabled: false.
func TestHandleMergeCmd_WriteDisabled(t *testing.T) {
	store := NewPlanStore(t.TempDir())
	r, _ := newScriptedREPL(t, store)
	r.writeEnabled = false
	out, _ := captureOut(r)

	r.handleMergeCmd("/merge")

	got := out.String()
	if !strings.Contains(strings.ToLower(got), "disabled") &&
		!strings.Contains(got, "write_enabled") {
		t.Errorf("expected L2 gate hint; got: %q", got)
	}
}

// gitInDir is a thin test helper that runs git with cwd=dir and
// fails the test on error. Identifier is distinct from the
// worktree package's helper to avoid cross-package symbol leakage.
func gitInDir(t *testing.T, dir string, args ...string) {
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

// captureOut reaches into the REPL's out writer (a *bytes.Buffer
// installed by newScriptedREPL) and returns it directly. Tests use
// this to assert on printed text.
func captureOut(r *REPL) (interface{ String() string }, bool) {
	if buf, ok := r.out.(interface{ String() string }); ok {
		return buf, true
	}
	return nil, false
}
