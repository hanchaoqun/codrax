package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PIB-W W-1 (ledger docs/design/pi_borrow_analysis_20260729.md §7.2):
// /revise is the third approval arm — settle the superseded plan for
// audit, keep the worktree and workflow alive, send the batch back to
// ReadyToPlan with the operator's feedback as a typed
// WriteWorkflowRevision.

// TestRevise_RequiresFeedback pins the usage contract: a bare /revise
// must not settle anything — feedback is required, /reject is the
// no-feedback arm.
func TestRevise_RequiresFeedback(t *testing.T) {
	runner := &writeCapableRunner{}
	r, store, out := newApprovalREPL(t, "", runner)
	originalPath := r.pendingPlanPath

	r.handleReviseCmd("/revise")

	if r.pendingPlanPath != originalPath {
		t.Errorf("bare /revise must preserve pendingPlanPath; got %q", r.pendingPlanPath)
	}
	plan, err := store.Load("plan-approve-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if plan.Status != "pending_approval" {
		t.Errorf("bare /revise must not settle the plan; status = %q", plan.Status)
	}
	if !strings.Contains(out.String(), "Usage: /revise") {
		t.Errorf("bare /revise must print usage; got: %q", out.String())
	}
}

// TestRevise_SettlesPlanAndMarksBatchReadyToPlan pins the full arm:
// plan settled rejected with the "revise: " audit prefix, active batch
// back to ReadyToPlan (NOT Blocked — the resume machinery must be able
// to pick it up), typed revision appended with empty ConsumedBy, and
// the progress ledger records reason "revision_requested".
func TestRevise_SettlesPlanAndMarksBatchReadyToPlan(t *testing.T) {
	runner := &writeCapableRunner{}
	r, store, _ := newApprovalREPL(t, "", runner)
	plan, err := store.Load("plan-approve-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	workflowStore := NewWriteWorkflowRunStore(store.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-revise-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-revise-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-revise-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: plan.ID,
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	r.writeWorkflowRunStore = workflowStore

	r.handleReviseCmd("/revise only touch the renderer, not the scheduler")

	settled, err := store.Load("plan-approve-1")
	if err != nil {
		t.Fatalf("Load settled: %v", err)
	}
	if settled.Status != types.PlanStatusRejected {
		t.Errorf("superseded plan status = %q, want %q (audit trail)", settled.Status, types.PlanStatusRejected)
	}
	if !strings.HasPrefix(settled.RejectionReason, "revise: ") {
		t.Errorf("audit reason must carry the revise prefix; got %q", settled.RejectionReason)
	}
	if r.pendingPlanPath != "" {
		t.Errorf("pendingPlanPath should be cleared after /revise; got %q", r.pendingPlanPath)
	}

	run, err := workflowStore.Load("wf-revise-1")
	if err != nil || run == nil {
		t.Fatalf("Load workflow: %v", err)
	}
	batch := run.Batches[0]
	if batch.Status != types.WriteWorkflowBatchReadyToPlan {
		t.Errorf("batch status = %q, want %q (resumable, not Blocked)", batch.Status, types.WriteWorkflowBatchReadyToPlan)
	}
	rev := types.WriteWorkflowBatchPendingRevision(batch)
	if rev == nil {
		t.Fatal("batch must carry a pending WriteWorkflowRevision after /revise")
	}
	if rev.Feedback != "only touch the renderer, not the scheduler" {
		t.Errorf("revision feedback = %q, want operator verbatim text", rev.Feedback)
	}
	if rev.PlanID != plan.ID {
		t.Errorf("revision PlanID = %q, want superseded plan id %q", rev.PlanID, plan.ID)
	}
	// The ledger must record the revision request, and — because
	// ReadyToPlan is resume-eligible — the immediate resume /revise
	// triggers appends its own entry AFTER it. Pinning the order
	// proves the third arm is not a Blocked dead end.
	revisionAt := -1
	resumedAfter := false
	for i, entry := range run.ProgressLedger {
		if entry.ReasonCode == "revision_requested" {
			revisionAt = i
		}
		if revisionAt >= 0 && i > revisionAt && entry.ReasonCode == "resumed" {
			resumedAfter = true
		}
	}
	if revisionAt < 0 {
		t.Fatalf("progress ledger must record revision_requested; got %+v", run.ProgressLedger)
	}
	if !resumedAfter {
		t.Errorf("expected the immediate resume to land after revision_requested; ledger: %+v", run.ProgressLedger)
	}
}

// TestRevise_NextActionCardOffersReviseArm pins the typed card: a
// pending-approval batch must offer revise_batch as the first
// secondary action (less destructive arm before terminal reject).
func TestRevise_NextActionCardOffersReviseArm(t *testing.T) {
	view := types.DeriveWriteWorkflowNextActionView(types.WriteWorkflowRun{
		RunID:         "wf-card",
		ActiveBatchID: "b1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "b1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-x",
		}},
	})
	if view.State != types.WriteWorkflowNextNeedsApproval {
		t.Fatalf("state = %q, want needs_approval", view.State)
	}
	if len(view.SecondaryActions) < 2 ||
		view.SecondaryActions[0] != types.WriteWorkflowNextActionReviseBatch ||
		view.SecondaryActions[1] != types.WriteWorkflowNextActionRejectBatch {
		t.Errorf("secondary actions = %v, want [revise_batch reject_batch ...]", view.SecondaryActions)
	}
}
