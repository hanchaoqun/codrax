package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// worktree_base_stamp_test.go — V5-1 (§40.35 复核二): the analysis base is a
// recorded fact, never a branch-topology guess. At provisioning it is the
// worktree's HEAD plus the main checkout's dirty roster; a plan that already
// records a base (applied later than planned, or a preserved worktree being
// re-verified) keeps its own base when that commit exists.

func stampTestRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "user.email", "codrax-test@example.invalid")
	git("config", "user.name", "Codrax Test")
	if err := os.WriteFile(filepath.Join(root, "cfg.py"), []byte("l1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "cfg.py")
	git("commit", "-q", "-m", "base")
	return root
}

func TestStampWorktreeBaseRecordsHeadAndDirtyRoster(t *testing.T) {
	root := stampTestRepo(t)
	if err := os.WriteFile(filepath.Join(root, "cfg.py"), []byte("l1\nl2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := &Orchestrator{busCtx: &types.BusContext{MainRepoRoot: root, Mutable: types.NewMutableState("x")}}
	o.stampWorktreeBase(root)
	if len(o.busCtx.WorktreeBaseSHA) != 40 || len(o.busCtx.WorktreeBaseDirtyPaths) != 1 || o.busCtx.WorktreeBaseDirtyPaths[0] != "cfg.py" {
		t.Fatalf("base/dirty roster = %q %v", o.busCtx.WorktreeBaseSHA, o.busCtx.WorktreeBaseDirtyPaths)
	}
	head := o.busCtx.WorktreeBaseSHA
	// A plan that recorded its own (existing) base keeps it.
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "p", WorktreeBaseSHA: head, WorktreeBaseDirtyPaths: []string{"other.py"}})
	o.stampWorktreeBase(root)
	if o.busCtx.WorktreeBaseSHA != head || len(o.busCtx.WorktreeBaseDirtyPaths) != 1 || o.busCtx.WorktreeBaseDirtyPaths[0] != "other.py" {
		t.Fatalf("plan-recorded base must win: %q %v", o.busCtx.WorktreeBaseSHA, o.busCtx.WorktreeBaseDirtyPaths)
	}
	// A plan whose recorded base is not a commit here falls back to HEAD.
	o.busCtx.Mutable.SetChangePlan(&types.ChangePlan{ID: "p", WorktreeBaseSHA: "0123456789abcdef0123456789abcdef01234567"})
	o.stampWorktreeBase(root)
	if o.busCtx.WorktreeBaseSHA != head {
		t.Fatalf("unknown recorded base must fall back to the worktree HEAD: %q", o.busCtx.WorktreeBaseSHA)
	}
	// The plan persists the base beside its worktree path.
	plan := &types.ChangePlan{ID: "p"}
	o.stampPlanWorktreeBase(plan)
	if plan.WorktreeBaseSHA != head || len(plan.WorktreeBaseDirtyPaths) != 1 {
		t.Fatalf("plan stamp = %+v", plan)
	}
	if types.PlanFingerprint(plan) != types.PlanFingerprint(&types.ChangePlan{ID: "p"}) {
		t.Fatal("the base is lifecycle metadata and must not enter the plan fingerprint")
	}
}
