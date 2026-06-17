package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
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

func TestMarkWorkflowRunActiveSliceApplying(t *testing.T) {
	run := onlineSliceRunForTest()
	run.Batches[0].Status = types.WriteWorkflowBatchApplying
	run.Batches[0].Slices[1].Status = types.ChangePlanSlicePending
	run.Batches[0].Slices[1].Completion = &types.WriteWorkflowCompletion{
		Verdict:    types.WriteWorkflowCompletionUnverified,
		ReasonCode: "stale",
	}
	plan := onlineSlicePlanForTest("plan-slice")

	markWorkflowRunActiveSliceApplying(&run, "batch-1", plan, "controller_apply")
	slice := run.Batches[0].Slices[1]
	if slice.Status != types.ChangePlanSliceApplying {
		t.Fatalf("slice status = %q, want applying", slice.Status)
	}
	if slice.Completion != nil {
		t.Fatalf("applying slice should clear stale completion: %+v", slice.Completion)
	}
	if len(slice.Attempts) != 1 || slice.Attempts[0].Kind != "apply" ||
		slice.Attempts[0].Status != "started" || slice.Attempts[0].ReasonCode != "controller_apply" {
		t.Fatalf("apply-start attempt not recorded: %+v", slice.Attempts)
	}
	if len(run.Batches[0].SliceEvents) != 1 ||
		run.Batches[0].SliceEvents[0].Event != types.WriteWorkflowSliceEventApplyStarted ||
		run.Batches[0].SliceEvents[0].Status != types.ChangePlanSliceApplying {
		t.Fatalf("apply-start event not recorded: %+v", run.Batches[0].SliceEvents)
	}
}

func TestUpdateWorkflowRunActiveSliceApplyRecordsCheckpoint(t *testing.T) {
	run := onlineSliceRunForTest()
	plan := onlineSlicePlanForTest("plan-slice")
	plan.AppliedCommitSHA = "abc123"
	plan.WorktreePath = "/tmp/wt"

	updateWorkflowRunBatchApply(&run, "batch-1", plan, "applied", "apply_succeeded")
	slice := run.Batches[0].Slices[1]
	if slice.Checkpoint == nil {
		t.Fatalf("expected slice checkpoint, got %+v", slice)
	}
	if slice.Checkpoint.Ref != "refs/codrax/applied/plan-slice" ||
		slice.Checkpoint.CommitSHA != "abc123" ||
		slice.Checkpoint.WorktreePath != "/tmp/wt" ||
		slice.Checkpoint.Source != "slice_apply" ||
		slice.Checkpoint.ReasonCode != "apply_succeeded" {
		t.Fatalf("checkpoint metadata wrong: %+v", slice.Checkpoint)
	}
}

func TestStampActivePlanCheckpointMetadata(t *testing.T) {
	o := &Orchestrator{
		busCtx:               &types.BusContext{WorktreePath: "/tmp/wt"},
		currentIterCommitSHA: "abc123",
	}
	plan := onlineSlicePlanForTest("plan-slice")
	got := o.stampActivePlanCheckpointMetadata(plan)
	if got.AppliedCommitSHA != "abc123" || got.WorktreePath != "/tmp/wt" {
		t.Fatalf("checkpoint metadata not stamped: %+v", got)
	}
}

func TestInitializeWorkflowBatchSlicesFromPlanResetsPriorPlanState(t *testing.T) {
	batch := types.WriteWorkflowBatch{
		ID:            "batch-1",
		PlanID:        "plan-old",
		ActiveSliceID: "slice-001",
		Slices: []types.WriteWorkflowSlice{{
			ID:            "slice-001",
			Status:        types.ChangePlanSliceFailed,
			PlanID:        "plan-old",
			ChangeIndexes: []int{0},
			Paths:         []string{"old.go"},
		}},
		SliceEvents: []types.WriteWorkflowSliceEvent{{
			SliceID: "slice-001",
			Event:   types.WriteWorkflowSliceEventFailed,
			Status:  types.ChangePlanSliceFailed,
			PlanID:  "plan-old",
		}},
	}
	plan := onlineSlicePlanForTest("plan-new")
	initializeWorkflowBatchSlicesFromPlan(&batch, plan, "plan-old")
	if batch.ActiveSliceID != "slice-001" {
		t.Fatalf("active slice = %q, want slice-001", batch.ActiveSliceID)
	}
	if len(batch.Slices) != 2 || batch.Slices[0].Status != types.ChangePlanSlicePending {
		t.Fatalf("new plan slices should reset to pending, got %+v", batch.Slices)
	}
	if batch.Slices[0].PlanID != "plan-new" || batch.Slices[0].Paths[0] != "a.go" {
		t.Fatalf("new slice metadata not derived from new plan: %+v", batch.Slices[0])
	}
}

func TestAdvanceWorkflowAfterSuccessfulSliceObserveAdvancesNextSlice(t *testing.T) {
	plan := onlineSlicePlanForTest("plan-slice")
	run := types.WriteWorkflowRun{
		RunID:         "wf",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Status:        types.WriteWorkflowBatchVerifying,
			PlanID:        "plan-slice",
			ActiveSliceID: "slice-001",
			Slices: []types.WriteWorkflowSlice{{
				ID:            "slice-001",
				Status:        types.ChangePlanSliceVerified,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{0},
				Paths:         []string{"a.go"},
			}, {
				ID:            "slice-002",
				Status:        types.ChangePlanSlicePending,
				PlanID:        "plan-slice",
				ChangeIndexes: []int{1},
				Paths:         []string{"b.go"},
			}},
		}},
	}
	mu := types.NewMutableState("test")
	mu.SetChangePlan(plan)
	mu.SetWriteWorkflowRun(&run)
	mu.SetChangeReport(&types.ChangeReport{PlanID: "plan-slice", Passed: true})
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu}}

	handled := o.advanceWorkflowAfterSuccessfulSliceObserve(&run, writeflow.VerifyAttemptOutcome{
		Kind:       writeflow.VerifyOutcomeReportPassed,
		ReasonCode: "tests_passed",
	})
	if !handled {
		t.Fatal("expected online slice transition to handle verified active slice")
	}
	if run.Batches[0].ActiveSliceID != "slice-002" || run.Batches[0].Status != types.WriteWorkflowBatchPlanned {
		t.Fatalf("run did not advance to next slice: %+v", run.Batches[0])
	}
	if got := mu.ChangePlan(); got == nil || got.Status != types.PlanStatusPending {
		t.Fatalf("plan should be reset to pending for next active slice, got %+v", got)
	}
	if mu.ChangeReport() != nil {
		t.Fatalf("change report should be cleared before next slice apply, got %+v", mu.ChangeReport())
	}
	pending := mu.WriteClosure().PendingApplies()
	if len(pending) != 1 || pending[0].Path != "b.go" || pending[0].Origin != "online_slice" {
		t.Fatalf("pending queue = %+v, want only next slice path b.go", pending)
	}
}

func TestAdvanceWorkflowAfterSuccessfulSliceObserveCompletesBatch(t *testing.T) {
	plan := onlineSlicePlanForTest("plan-slice")
	run := onlineSliceRunForTest()
	run.Batches[0].Slices[1].Status = types.ChangePlanSliceVerified
	run.Batches[0].Slices[1].Completion = &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionVerified}
	mu := types.NewMutableState("test")
	mu.SetChangePlan(plan)
	mu.SetWriteWorkflowRun(&run)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu}}

	handled := o.advanceWorkflowAfterSuccessfulSliceObserve(&run, writeflow.VerifyAttemptOutcome{
		Kind:       writeflow.VerifyOutcomeReportPassed,
		ReasonCode: "tests_passed",
	})
	if !handled {
		t.Fatal("expected online slice transition to complete final verified slice")
	}
	if run.Batches[0].Status != types.WriteWorkflowBatchComplete {
		t.Fatalf("batch status = %q, want complete", run.Batches[0].Status)
	}
	if run.Batches[0].Completion == nil || run.Batches[0].Completion.Verdict != types.WriteWorkflowCompletionVerified {
		t.Fatalf("batch completion not verified: %+v", run.Batches[0].Completion)
	}
}

func TestMarkWorkflowRunActiveSliceObservingForRestoredPlan(t *testing.T) {
	run := onlineSliceRunForTest()
	run.Batches[0].Slices[1].Status = types.ChangePlanSliceFailed
	plan := onlineSlicePlanForTest("plan-slice")
	plan.AppliedCommitSHA = "sha"
	plan.WorktreePath = "/tmp/wt"

	markWorkflowRunActiveSliceObservingForRestoredPlan(&run, "batch-1", plan, "planner_probe_passed_existing_worktree")
	slice := run.Batches[0].Slices[1]
	if slice.Status != types.ChangePlanSliceObserving {
		t.Fatalf("slice status = %q, want observing", slice.Status)
	}
	if len(run.Batches[0].SliceEvents) != 1 ||
		run.Batches[0].SliceEvents[0].Event != types.WriteWorkflowSliceEventObserveStarted {
		t.Fatalf("observe-start event not recorded: %+v", run.Batches[0].SliceEvents)
	}
	if slice.Restore == nil || slice.Restore.CheckpointRef != "refs/codrax/applied/plan-slice" ||
		slice.Restore.CommitSHA != "sha" || slice.Restore.WorktreePath != "/tmp/wt" ||
		slice.Restore.ReasonCode != "planner_probe_passed_existing_worktree" {
		t.Fatalf("restore metadata not recorded: %+v", slice.Restore)
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

func onlineSlicePlanForTest(id string) *types.ChangePlan {
	return &types.ChangePlan{
		ID:          id,
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"a.go", "b.go"},
		Changes: []types.FileChange{{
			Path:      "a.go",
			Kind:      "modify",
			Rationale: "a",
		}, {
			Path:      "b.go",
			Kind:      "modify",
			Rationale: "b",
		}},
		Slices: []types.ChangePlanSlice{{
			ID:            "slice-001",
			Status:        types.ChangePlanSlicePending,
			ChangeIndexes: []int{0},
		}, {
			ID:            "slice-002",
			Status:        types.ChangePlanSlicePending,
			ChangeIndexes: []int{1},
		}},
	}
}
