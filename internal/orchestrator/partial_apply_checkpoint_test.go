package orchestrator

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PIB-W2 W-3 (ledger docs/design/pi_borrow_analysis_20260729.md §7.2):
// a partially_applied failure commits the landed units as a Partial
// checkpoint so their bytes survive the failure-path worktree discard
// — recovery/audit anchor only, never a deliverable.

func TestCommitPartialApplyCheckpoint_PreservesLandedUnits(t *testing.T) {
	work := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@t")
	git("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(work, "a.go"), []byte("base\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "base")
	// The landed unit: a.go was applied before the batch failed on b.go.
	if err := os.WriteFile(filepath.Join(work, "a.go"), []byte("applied unit\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	mut := types.NewMutableState("")
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-partial-1", TargetPaths: []string{"a.go", "b.go"}})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mut, WorktreePath: work}}

	o.commitPartialApplyCheckpoint([]string{"a.go"})

	plan := mut.ChangePlan()
	cp := plan.ApplyCheckpoint
	if cp == nil || !cp.Partial {
		t.Fatalf("expected a Partial checkpoint record; got %+v", cp)
	}
	if cp.CommitSHA == "" || cp.CommitError != "" {
		t.Fatalf("checkpoint commit must succeed; got %+v", cp)
	}
	// Second lock: the mergeable success anchor stays unset.
	if plan.AppliedCommitSHA != "" {
		t.Fatal("partial lane must NOT set plan.AppliedCommitSHA (the /merge success anchor)")
	}
	// The landed bytes are durable inside the commit.
	show := exec.Command("git", "show", cp.CommitSHA+":a.go")
	show.Dir = work
	out, err := show.Output()
	if err != nil || !strings.Contains(string(out), "applied unit") {
		t.Fatalf("landed bytes not durable in checkpoint commit: %q err=%v", out, err)
	}

	// Guard arms: no worktree / no applied paths / no plan → no-op.
	o2 := &Orchestrator{busCtx: &types.BusContext{Mutable: types.NewMutableState("")}}
	o2.commitPartialApplyCheckpoint([]string{"a.go"}) // must not panic
}

// TestWorkflowActiveBatchLastApplyFailed pins the replan reason-code
// discriminator: the last apply attempt's status decides whether a
// needs-replan restore is labeled apply_failed_replan.
func TestWorkflowActiveBatchLastApplyFailed(t *testing.T) {
	run := &types.WriteWorkflowRun{
		ActiveBatchID: "b1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "b1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "plan", Status: "complete"},
				{Kind: "apply", Status: "applied"},
				{Kind: "verify", Status: "failed"},
			},
		}},
	}
	if workflowActiveBatchLastApplyFailed(run) {
		t.Fatal("last apply attempt succeeded → verify lane, not apply lane")
	}
	run.Batches[0].Attempts = append(run.Batches[0].Attempts,
		types.WriteWorkflowAttempt{Kind: "apply", Status: "partial"})
	if !workflowActiveBatchLastApplyFailed(run) {
		t.Fatal("a partial apply attempt must select the apply_failed_replan label")
	}
	if workflowActiveBatchLastApplyFailed(nil) || workflowActiveBatchLastApplyFailed(&types.WriteWorkflowRun{}) {
		t.Fatal("nil/empty runs must be false")
	}
}
