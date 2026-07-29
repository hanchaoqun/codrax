package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// PIB-W W-1 (ledger docs/design/pi_borrow_analysis_20260729.md §7.2):
// planner-side consumption of a pending /revise — the hint quotes the
// operator's feedback verbatim as data and names the superseded plan;
// minting a replacement plan stamps ConsumedBy so stale feedback never
// re-injects into later replans.

func TestAppendOperatorRevisionHint_QuotesFeedbackAndNamesPlan(t *testing.T) {
	var b strings.Builder
	appendOperatorRevisionHint(&b, &types.WriteWorkflowRevision{
		PlanID:   "plan-old-7",
		Feedback: `only touch the renderer, keep "scheduler" untouched`,
	})
	got := b.String()
	for _, want := range []string{
		"supersedes plan plan-old-7",
		`"only touch the renderer, keep \"scheduler\" untouched"`, // %q-quoted data, not paraphrased
		"reviewed by the operator again",                          // declared downstream reader
	} {
		if !strings.Contains(got, want) {
			t.Errorf("revision hint missing %q:\n%s", want, got)
		}
	}

	// Nil revision appends nothing — the common no-revision path must
	// stay byte-identical.
	var empty strings.Builder
	appendOperatorRevisionHint(&empty, nil)
	if empty.Len() != 0 {
		t.Errorf("nil revision must append nothing; got %q", empty.String())
	}
}

func TestPendingActiveBatchRevision_ActiveBatchOnly(t *testing.T) {
	run := &types.WriteWorkflowRun{
		ActiveBatchID: "b2",
		Batches: []types.WriteWorkflowBatch{
			{ID: "b1", Revisions: []types.WriteWorkflowRevision{{Feedback: "stale other-batch feedback"}}},
			{ID: "b2", Revisions: []types.WriteWorkflowRevision{
				{Feedback: "consumed", ConsumedBy: "plan-new-1"},
				{Feedback: "live feedback"},
			}},
		},
	}
	rev := pendingActiveBatchRevision(run)
	if rev == nil || rev.Feedback != "live feedback" {
		t.Fatalf("pending revision = %+v, want the active batch's unconsumed entry", rev)
	}
	if pendingActiveBatchRevision(nil) != nil {
		t.Error("nil run must yield nil revision")
	}
	run.ActiveBatchID = ""
	if pendingActiveBatchRevision(run) != nil {
		t.Error("no active batch must yield nil revision")
	}
}

// TestUpdateWorkflowRunBatchPlan_ConsumesPendingRevisions pins the
// consumption stamp: binding a freshly minted plan to the batch marks
// every pending revision ConsumedBy=<new plan id>, and a later
// pending-revision probe returns nil.
func TestUpdateWorkflowRunBatchPlan_ConsumesPendingRevisions(t *testing.T) {
	run := &types.WriteWorkflowRun{
		ActiveBatchID: "b1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "b1",
			Status: types.WriteWorkflowBatchReadyToPlan,
			Revisions: []types.WriteWorkflowRevision{
				{PlanID: "plan-old", Feedback: "tighten scope"},
			},
		}},
	}
	plan := &types.ChangePlan{ID: "plan-new-9", Status: types.PlanStatusPending}
	updateWorkflowRunBatchPlan(run, "b1", plan)

	if got := run.Batches[0].Revisions[0].ConsumedBy; got != "plan-new-9" {
		t.Errorf("ConsumedBy = %q, want the minted plan id", got)
	}
	if pendingActiveBatchRevision(run) != nil {
		t.Error("after consumption there must be no pending revision")
	}
	// History stays append-only: the consumed entry is retained.
	if len(run.Batches[0].Revisions) != 1 || run.Batches[0].Revisions[0].Feedback != "tighten scope" {
		t.Errorf("revision history must be retained; got %+v", run.Batches[0].Revisions)
	}
}
