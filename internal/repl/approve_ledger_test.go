package repl

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PIB-W W-2 (ledger docs/design/pi_borrow_analysis_20260729.md §7.2):
// approval decisions land on the run's append-only ledger and the
// /workflow show approval block surfaces the typed risk Reasons[] that
// were previously only visible at the pause moment.

// TestApprove_AppendsRunApprovalLedger pins the REPL stamp site: a
// confirmed /approve must append the WriteApprovalRecord to the active
// run's ApprovalRecords (value copy, decision order).
func TestApprove_AppendsRunApprovalLedger(t *testing.T) {
	runner := &writeCapableRunner{}
	r, store, _ := newApprovalREPL(t, "y\n", runner)
	plan, err := store.Load("plan-approve-1")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	workflowStore := NewWriteWorkflowRunStore(store.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-ledger-1",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-ledger-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-ledger-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: plan.ID,
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	r.writeWorkflowRunStore = workflowStore

	r.handleApproveCmd("/approve")

	run, err := workflowStore.Load("wf-ledger-1")
	if err != nil || run == nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if len(run.ApprovalRecords) != 1 {
		t.Fatalf("ApprovalRecords len = %d, want 1", len(run.ApprovalRecords))
	}
	rec := run.ApprovalRecords[0]
	if rec.UserDecision != "approved" || rec.Source != "repl_approve" {
		t.Errorf("ledger record = user=%q source=%q, want approved/repl_approve", rec.UserDecision, rec.Source)
	}
	if rec.PlanFingerprint == "" {
		t.Error("ledger record must carry the plan fingerprint")
	}
}

// TestAppendWriteWorkflowApprovalRecord_AppendOnlyValueCopy pins the
// helper contract: nil-safe, appends values (later mutation of the
// source record must not rewrite history).
func TestAppendWriteWorkflowApprovalRecord_AppendOnlyValueCopy(t *testing.T) {
	types.AppendWriteWorkflowApprovalRecord(nil, &types.WriteApprovalRecord{}) // must not panic
	run := &types.WriteWorkflowRun{}
	types.AppendWriteWorkflowApprovalRecord(run, nil) // must not panic
	if len(run.ApprovalRecords) != 0 {
		t.Fatal("nil record must not append")
	}
	rec := &types.WriteApprovalRecord{UserDecision: "approved", RiskLevel: "high"}
	types.AppendWriteWorkflowApprovalRecord(run, rec)
	rec.UserDecision = "mutated-after-append"
	if run.ApprovalRecords[0].UserDecision != "approved" {
		t.Error("ledger must store a value copy; post-append mutation leaked into history")
	}
}

// TestWorkflowApprovalLines_SurfacesReasonsAndLedgerCount pins the
// /workflow show approval block: risk Reasons[] render (cap 4 + rollup
// line) and the append-only ledger count is disclosed.
func TestWorkflowApprovalLines_SurfacesReasonsAndLedgerCount(t *testing.T) {
	plan := &types.ChangePlan{
		ID:     "plan-show-1",
		Status: types.PlanStatusPending,
		Approval: &types.WriteApprovalRecord{
			Policy:       "auto_safe",
			RiskLevel:    "high",
			Action:       "manual_approval",
			UserDecision: "pending",
			Reasons: []types.WriteApprovalReason{
				{Code: "path_outside_allowlist", Detail: "touches cmd/root.go", Level: "high"},
				{Code: "large_diff", Detail: "412 lines", Level: "medium"},
				{Code: "r3", Level: "low"},
				{Code: "r4", Level: "low"},
				{Code: "r5-overflow", Level: "low"},
			},
		},
	}
	run := types.WriteWorkflowRun{
		RunID:         "wf-show",
		ActiveBatchID: "b1",
		ApprovalRecords: []types.WriteApprovalRecord{
			{UserDecision: "manual_required"},
			{UserDecision: "approved"},
		},
	}
	batch := types.WriteWorkflowBatch{ID: "b1", PlanID: plan.ID, Status: types.WriteWorkflowBatchPendingApproval}

	var b strings.Builder
	writeWorkflowApprovalLines(&b, "en", run, batch, true, plan)
	got := b.String()
	for _, want := range []string{
		"[high] path_outside_allowlist — touches cmd/root.go",
		"[medium] large_diff — 412 lines",
		"… 1 more reason(s)",
		"2 approval decision(s) recorded for this run (append-only)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("approval block missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "r5-overflow") {
		t.Errorf("beyond-cap reason must roll up, not render:\n%s", got)
	}
}
