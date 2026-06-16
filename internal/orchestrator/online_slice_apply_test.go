package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPendingAppliesForActivePlanScopeUsesActiveSlice(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-slice",
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create", Rationale: "a"},
			{Path: "b.go", Kind: "create", Rationale: "b"},
			{Path: "c.go", Kind: "create", Rationale: "c"},
		},
		TargetPaths: []string{"a.go", "b.go", "c.go"},
	}
	mu := types.NewMutableState("test")
	mu.SetWriteWorkflowRun(&types.WriteWorkflowRun{
		RunID:         "wf",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchApplying,
			ActiveSliceID: "slice-002",
			Slices: []types.WriteWorkflowSlice{{
				ID:            "slice-001",
				Status:        types.ChangePlanSliceVerified,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{0},
			}, {
				ID:            "slice-002",
				Status:        types.ChangePlanSliceApplying,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{1},
			}},
		}},
	})
	pending, active := pendingAppliesForActivePlanScope(mu, plan, "test")
	if !active {
		t.Fatal("expected active slice pending scope")
	}
	if len(pending) != 1 || pending[0].Path != "b.go" || pending[0].Origin != "test" {
		t.Fatalf("pending = %+v, want only b.go from active slice", pending)
	}
}

func TestPendingAppliesForActivePlanScopeFallsBackToFullPlan(t *testing.T) {
	plan := &types.ChangePlan{
		ID: "plan-full",
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create", Rationale: "a"},
			{Path: "b.go", Kind: "create", Rationale: "b"},
		},
		TargetPaths: []string{"a.go", "b.go"},
	}
	pending, active := pendingAppliesForActivePlanScope(types.NewMutableState("test"), plan, "test")
	if active {
		t.Fatal("full-plan fallback should not report active slice")
	}
	if len(pending) != 2 || pending[0].Path != "a.go" || pending[1].Path != "b.go" {
		t.Fatalf("pending = %+v, want full plan order", pending)
	}
}

func TestUpdateWorkflowRunActiveSliceObservePassed(t *testing.T) {
	run := onlineSliceRunForTest()
	report := &types.ChangeReport{
		PlanID:             "plan-slice",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
	}
	updateWorkflowRunActiveSliceObserve(&run, "batch-1", report, "passed", "tests_passed")
	slice := run.Batches[0].Slices[1]
	if slice.Status != types.ChangePlanSliceVerified {
		t.Fatalf("slice status = %q, want verified", slice.Status)
	}
	if slice.ObserveRef != "plan-slice.report.json" || slice.VerifyRef != "plan-slice.report.json" {
		t.Fatalf("slice report refs not set: %+v", slice)
	}
	if slice.Completion == nil || slice.Completion.Verdict != types.WriteWorkflowCompletionVerified {
		t.Fatalf("slice completion not verified: %+v", slice.Completion)
	}
	if len(slice.Attempts) != 1 || slice.Attempts[0].Kind != "observe" || slice.Attempts[0].Status != "passed" {
		t.Fatalf("observe attempt not recorded: %+v", slice.Attempts)
	}
	if len(run.Batches[0].SliceEvents) != 2 ||
		run.Batches[0].SliceEvents[0].Event != types.WriteWorkflowSliceEventObserveCompleted ||
		run.Batches[0].SliceEvents[1].Event != types.WriteWorkflowSliceEventVerified {
		t.Fatalf("slice events not recorded: %+v", run.Batches[0].SliceEvents)
	}
}

func TestUpdateWorkflowRunActiveSliceObserveUnavailable(t *testing.T) {
	run := onlineSliceRunForTest()
	report := &types.ChangeReport{
		PlanID:             "plan-slice",
		VerificationStatus: types.VerificationStatusUnavailable,
		FailureKind:        types.FailureKindRunnerMissing,
	}
	updateWorkflowRunActiveSliceObserve(&run, "batch-1", report, "unverified", string(types.FailureKindRunnerMissing))
	slice := run.Batches[0].Slices[1]
	if slice.Status != types.ChangePlanSliceUnverified {
		t.Fatalf("slice status = %q, want unverified", slice.Status)
	}
	if slice.Completion == nil || slice.Completion.Verdict != types.WriteWorkflowCompletionUnverified {
		t.Fatalf("slice completion not unverified: %+v", slice.Completion)
	}
	if len(run.Batches[0].SliceEvents) != 2 ||
		run.Batches[0].SliceEvents[1].Event != types.WriteWorkflowSliceEventUnverified {
		t.Fatalf("slice unverified event not recorded: %+v", run.Batches[0].SliceEvents)
	}
}

func TestUpdateWorkflowRunActiveSliceObserveFailed(t *testing.T) {
	run := onlineSliceRunForTest()
	report := &types.ChangeReport{
		PlanID:             "plan-slice",
		Passed:             false,
		VerificationStatus: types.VerificationStatusFailed,
		FailureKind:        types.FailureKindTestsFailed,
	}
	updateWorkflowRunActiveSliceObserve(&run, "batch-1", report, "failed", "tests_failed")
	slice := run.Batches[0].Slices[1]
	if slice.Status != types.ChangePlanSliceFailed {
		t.Fatalf("slice status = %q, want failed", slice.Status)
	}
	if slice.Completion != nil {
		t.Fatalf("failed slice should not carry accepted completion: %+v", slice.Completion)
	}
	if len(run.Batches[0].SliceEvents) != 2 ||
		run.Batches[0].SliceEvents[1].Event != types.WriteWorkflowSliceEventFailed {
		t.Fatalf("slice failed event not recorded: %+v", run.Batches[0].SliceEvents)
	}
}

func TestUpdateWorkflowRunActiveSliceObserveSkipped(t *testing.T) {
	run := onlineSliceRunForTest()
	updateWorkflowRunActiveSliceObserveSkipped(&run, "batch-1", "plan-slice")
	slice := run.Batches[0].Slices[1]
	if slice.Status != types.ChangePlanSliceUnverified {
		t.Fatalf("slice status = %q, want unverified", slice.Status)
	}
	if slice.Completion == nil || slice.Completion.ReasonCode != "skip_verify" {
		t.Fatalf("skipped observe completion not recorded: %+v", slice.Completion)
	}
	if len(run.Batches[0].SliceEvents) != 1 ||
		run.Batches[0].SliceEvents[0].Event != types.WriteWorkflowSliceEventUnverified {
		t.Fatalf("skipped event not recorded: %+v", run.Batches[0].SliceEvents)
	}
}

func onlineSliceRunForTest() types.WriteWorkflowRun {
	return types.WriteWorkflowRun{
		RunID:         "wf",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchVerifying,
			PlanID:        "plan-slice",
			ActiveSliceID: "slice-002",
			Slices: []types.WriteWorkflowSlice{{
				ID:            "slice-001",
				Status:        types.ChangePlanSliceVerified,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{0},
			}, {
				ID:            "slice-002",
				Status:        types.ChangePlanSliceObserving,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{1},
			}},
		}},
	}
}
