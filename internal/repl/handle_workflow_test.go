package repl

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestWorkflowShowDisplaysActiveWriteWorkflow(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:          "p1",
		Summary:     "show workflow",
		Status:      types.PlanStatusPending,
		TargetPaths: []string{"internal/repl/repl.go"},
		Approval: &types.WriteApprovalRecord{
			Policy:          "auto_safe",
			RiskLevel:       "high",
			Action:          "ask",
			UserDecision:    "pending",
			ReasonCode:      "high_risk",
			PlanFingerprint: "fp-show",
		},
	}
	if _, err := planStore.SaveForTest(plan); err != nil {
		t.Fatalf("SaveForTest: %v", err)
	}
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-show",
		Goal:          "ship workflow visibility",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Budget: types.WriteWorkflowBudget{
			MaxBatches:            5,
			MaxExplorationRounds:  2,
			BatchesUsed:           1,
			ExplorationRoundsUsed: 1,
		},
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-1",
			Goal:          "show active batch",
			Status:        types.WriteWorkflowBatchPendingApproval,
			PlanID:        "p1",
			ApplyRef:      "applied/p1",
			VerifyRef:     "p1.report.json",
			ActiveSliceID: "slice-002",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-001",
				Status: types.ChangePlanSliceVerified,
			}, {
				ID:     "slice-002",
				Status: types.ChangePlanSlicePending,
			}},
		}},
		ContextPacks: []types.WriteContextPack{{
			PackID:  "pack-1",
			BatchID: "batch-1",
			Items: []types.WriteContextItem{{
				Priority: types.WriteContextP0,
				Kind:     "constraint",
				Text:     "do not break read mode",
			}, {
				Priority: types.WriteContextP1,
				Kind:     "target_file",
				Text:     "internal/repl/repl.go",
			}},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-1",
			Stage:      "controller",
			ReasonCode: "pending_approval",
			Message:    "waiting for operator",
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                stubRunner{},
		In:                    strings.NewReader(""),
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
	})

	r.handleWorkflowCmd("/workflow show")

	got := out.String()
	for _, want := range []string{
		"Write workflow `wf-show`",
		"Active batch: `batch-1`",
		"plan `p1`",
		"apply `applied/p1`",
		"verify `p1.report.json`",
		"slice `slice-002` `pending` 1/2",
		"policy=`auto_safe` risk=`high` action=`ask`",
		"fingerprint=`fp-show`",
		"Handoff context: 1 pack(s), P0=1 P1=1",
		"do not break read mode",
		"Status: approval required",
		"Next: review risk and diff",
		"Advanced: /approve",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("workflow show missing %q:\n%s", want, got)
		}
	}
}

func TestWorkflowShowReadyToPlanDoesNotAskForApproval(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-ready",
		Goal:          "continue automatically",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                stubRunner{},
		In:                    strings.NewReader(""),
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
	})

	r.handleWorkflowCmd("/workflow show")

	got := out.String()
	if !strings.Contains(got, "Status: workflow is running; no user action is needed yet.") {
		t.Fatalf("ready_to_plan should render as running, got:\n%s", got)
	}
	for _, unexpected := range []string{"approval required", "/approve", "approve this plan"} {
		if strings.Contains(got, unexpected) {
			t.Fatalf("ready_to_plan must not ask for approval (%q):\n%s", unexpected, got)
		}
	}
}

func TestApproveUsesActiveWorkflowBatchPlan(t *testing.T) {
	runner := &capturingRunner{}
	planStore := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:      "plan-workflow-approve",
		Summary: "approve workflow batch",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{
			Path: "x.go",
			Kind: "modify",
		}},
		TargetPaths: []string{"x.go"},
		CreatedAt:   time.Now(),
	}
	planPath, err := planStore.SaveForTest(plan)
	if err != nil {
		t.Fatalf("SaveForTest: %v", err)
	}
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-approve",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-workflow-approve",
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { memStore.Close() })
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                runner,
		Store:                 memStore,
		In:                    strings.NewReader(""),
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
		WriteEnabled:          true,
	})

	r.handleApproveCmd("/approve")

	if !runner.runCalled {
		t.Fatal("/approve should dispatch the active workflow batch plan")
	}
	if runner.planPathAtRun != planPath {
		t.Fatalf("plan path at Run = %q, want %q", runner.planPathAtRun, planPath)
	}
	loaded, err := workflowStore.Load("wf-approve")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.Status != types.WriteWorkflowRunInProgress || loaded.Batches[0].Status != types.WriteWorkflowBatchComplete {
		t.Fatalf("approved batch should be complete without terminalizing run: %+v", loaded)
	}
}

func TestRejectMarksOnlyActiveWorkflowBatchBlocked(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:      "plan-workflow-reject",
		Summary: "reject workflow batch",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{
			Path: "x.go",
			Kind: "modify",
		}},
		TargetPaths: []string{"x.go"},
	}
	if _, err := planStore.SaveForTest(plan); err != nil {
		t.Fatalf("SaveForTest: %v", err)
	}
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-reject",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-workflow-reject",
		}, {
			ID:     "batch-2",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { memStore.Close() })
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                stubRunner{},
		Store:                 memStore,
		In:                    strings.NewReader(""),
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
	})

	r.handleRejectCmd("/reject scope wrong")

	rejected, err := planStore.Load("plan-workflow-reject")
	if err != nil {
		t.Fatalf("Load plan: %v", err)
	}
	if rejected.Status != types.PlanStatusRejected {
		t.Fatalf("plan status = %q, want rejected", rejected.Status)
	}
	loaded, err := workflowStore.Load("wf-reject")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.Status != types.WriteWorkflowRunInProgress {
		t.Fatalf("rejecting one batch must not block whole workflow: %+v", loaded)
	}
	if loaded.Batches[0].Status != types.WriteWorkflowBatchBlocked || loaded.Batches[1].Status != types.WriteWorkflowBatchReadyToPlan {
		t.Fatalf("only active batch should be blocked: %+v", loaded.Batches)
	}
	if len(loaded.ProgressLedger) == 0 || loaded.ProgressLedger[len(loaded.ProgressLedger)-1].ReasonCode != "rejected" {
		t.Fatalf("workflow rejection progress missing: %+v", loaded.ProgressLedger)
	}
}

func TestWorkflowResumeSelectsSavedWriteWorkflow(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-resume",
		Status:        types.WriteWorkflowRunPlanned,
		ActiveBatchID: "batch-done",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-done",
			Status: types.WriteWorkflowBatchComplete,
		}, {
			ID:     "batch-next",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                stubRunner{},
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
	})

	r.handleWorkflowCmd("/workflow resume wf-resume")

	loaded, err := workflowStore.Load("wf-resume")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.Status != types.WriteWorkflowRunInProgress || loaded.ActiveBatchID != "batch-next" {
		t.Fatalf("resume should activate next resumable batch: %+v", loaded)
	}
	if len(loaded.ProgressLedger) == 0 || loaded.ProgressLedger[len(loaded.ProgressLedger)-1].ReasonCode != "resumed" {
		t.Fatalf("resume progress missing: %+v", loaded.ProgressLedger)
	}
}

func TestWorkflowResumeThenApproveContinuesSamePendingApprovalRun(t *testing.T) {
	runner := &capturingRunner{}
	planStore := NewPlanStore(t.TempDir())
	plan := &types.ChangePlan{
		ID:      "plan-resume-approve",
		Summary: "resume and approve active batch",
		Status:  types.PlanStatusPending,
		Changes: []types.FileChange{{
			Path: "internal/repl/repl.go",
			Kind: "modify",
		}},
		TargetPaths: []string{"internal/repl/repl.go"},
		CreatedAt:   time.Now(),
	}
	assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
	decision := writeflow.DecideWriteApproval(writeflow.ApprovalPolicyManual, assessment)
	plan.Approval = writeflow.NewApprovalRecord(assessment, decision, "apply_pre_hook", "required", types.PlanFingerprint(plan), "operator")
	planPath, err := planStore.SaveForTest(plan)
	if err != nil {
		t.Fatalf("SaveForTest: %v", err)
	}
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-resume-approve",
		Goal:          "resume pending approval",
		Status:        types.WriteWorkflowRunPlanned,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:          "batch-1",
			Goal:        "approve current plan",
			Status:      types.WriteWorkflowBatchPendingApproval,
			PlanID:      "plan-resume-approve",
			ApprovalRef: "plan-resume-approve.json",
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}

	resumeOut := &bytes.Buffer{}
	resumeREPL := New(Config{
		Runner:                stubRunner{},
		In:                    strings.NewReader(""),
		Out:                   resumeOut,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
		WriteEnabled:          true,
	})
	resumeREPL.handleWorkflowCmd("/workflow resume wf-resume-approve")

	memStore, err := memory.NewStore(t.TempDir(), stubSummarizer{}, types.MemorySettings{})
	if err != nil {
		t.Fatalf("memory.NewStore: %v", err)
	}
	t.Cleanup(func() { memStore.Close() })
	approveOut := &bytes.Buffer{}
	approveREPL := New(Config{
		Runner:                runner,
		Store:                 memStore,
		In:                    strings.NewReader("y\n"),
		Out:                   approveOut,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
		WriteEnabled:          true,
		WriteApprovalPolicy:   writeflow.ApprovalPolicyManual,
	})

	approveREPL.handleApproveCmd("/approve")

	if !runner.runCalled {
		t.Fatal("/approve should dispatch the resumed workflow plan")
	}
	if runner.modeAtRun != types.ModeApply {
		t.Fatalf("mode at Run = %q, want apply", runner.modeAtRun)
	}
	if runner.planPathAtRun != planPath {
		t.Fatalf("plan path at Run = %q, want %q", runner.planPathAtRun, planPath)
	}
	approved, err := planStore.Load("plan-resume-approve")
	if err != nil {
		t.Fatalf("Load approved plan: %v", err)
	}
	if approved.Approval == nil || approved.Approval.UserDecision != "approved" {
		t.Fatalf("approval record should be persisted as approved: %+v", approved.Approval)
	}
	if !approved.ApprovalRecordIntegrityOK() || !approved.ApprovalMatchesCurrentFingerprint() {
		t.Fatalf("approval record should match current plan fingerprint: %+v", approved.Approval)
	}
	loaded, err := workflowStore.Load("wf-resume-approve")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.Status != types.WriteWorkflowRunInProgress || loaded.ActiveBatchID != "batch-1" {
		t.Fatalf("workflow should remain resumable on same run: %+v", loaded)
	}
	if loaded.Batches[0].Status != types.WriteWorkflowBatchComplete {
		t.Fatalf("approved active batch should be complete: %+v", loaded.Batches[0])
	}
	if len(loaded.ProgressLedger) < 2 ||
		loaded.ProgressLedger[len(loaded.ProgressLedger)-2].ReasonCode != "resumed" ||
		loaded.ProgressLedger[len(loaded.ProgressLedger)-1].ReasonCode != "approved_batch_complete" {
		t.Fatalf("resume and approval progress should both persist: %+v", loaded.ProgressLedger)
	}
}

func TestWorkflowListDisplaysSavedWriteWorkflowSnapshots(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-list-a",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-a",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-a",
			Status:        types.WriteWorkflowBatchPendingApproval,
			PlanID:        "plan-a",
			ActiveSliceID: "slice-a2",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-a1",
				Status: types.ChangePlanSliceVerified,
			}, {
				ID:     "slice-a2",
				Status: types.ChangePlanSlicePending,
			}},
		}},
		ContextPacks: []types.WriteContextPack{{
			PackID:  "pack-a",
			BatchID: "batch-a",
			Items: []types.WriteContextItem{{
				Priority: types.WriteContextP0,
				Kind:     "approval",
				Text:     "manual approval required",
			}},
		}},
	}); err != nil {
		t.Fatalf("Save workflow A: %v", err)
	}
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-list-b",
		Status:        types.WriteWorkflowRunBlocked,
		ActiveBatchID: "batch-b",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-b",
			Status: types.WriteWorkflowBatchBlocked,
		}},
	}); err != nil {
		t.Fatalf("Save workflow B: %v", err)
	}
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-list-c",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-c",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-c",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}); err != nil {
		t.Fatalf("Save workflow C: %v", err)
	}
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-list-d",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-d",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-d",
			Status: types.WriteWorkflowBatchPlanned,
			PlanID: "plan-d",
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-d",
			ReasonCode: "batch_planned",
		}},
	}); err != nil {
		t.Fatalf("Save workflow D: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                stubRunner{},
		In:                    strings.NewReader(""),
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
	})

	r.handleWorkflowCmd("/workflow list")

	got := out.String()
	compact := strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "", "│", "").Replace(got)
	for _, want := range []string{
		"Writeworkflows",
		"`wf-list-a`status=`in_progress`active=`batch-a`",
		"batch_status=`pending_approval`slice=`slice-a2:pending:1/2`next=`needs_approval`action=`approve_batch`user=true",
		"`wf-list-b`status=`blocked`active=`batch-b`",
		"batch_status=`blocked`slice=`none`next=`blocked`action=`inspect_evidence`user=true",
		"`wf-list-c`status=`in_progress`active=`batch-c`",
		"batch_status=`ready_to_plan`slice=`none`next=`running`action=`wait`user=false",
		"`wf-list-d`status=`in_progress`active=`batch-d`",
		"batch_status=`planned`slice=`none`next=`running`action=`wait`user=false",
		"packs=1",
		"packs=0",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("workflow list missing %q:\n%s", want, got)
		}
	}
}

func TestActiveWriteWorkflowBannerLineUsesTypedNextAction(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-banner-running",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-running",
		Batches: []types.WriteWorkflowBatch{{
			ID:            "batch-running",
			Status:        types.WriteWorkflowBatchPlanned,
			PlanID:        "plan-running",
			ActiveSliceID: "slice-002",
			Slices: []types.WriteWorkflowSlice{{
				ID:     "slice-001",
				Status: types.ChangePlanSliceVerified,
			}, {
				ID:     "slice-002",
				Status: types.ChangePlanSliceApplying,
			}},
		}},
		ProgressLedger: []types.WriteWorkflowProgress{{
			BatchID:    "batch-running",
			ReasonCode: "batch_planned",
		}},
	}); err != nil {
		t.Fatalf("Save running workflow: %v", err)
	}
	r := &REPL{
		language:              "en",
		writeWorkflowRunStore: workflowStore,
	}
	got := r.activeWriteWorkflowBannerLine()
	for _, want := range []string{"wf-banner-running", "running automatically", "batch-running", "slice `slice-002` `applying` 1/2", "no user action needed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("running workflow banner missing %q: %q", want, got)
		}
	}
	for _, blocked := range []string{"/workflow resume", "needs approval"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("running workflow banner must not suggest %q: %q", blocked, got)
		}
	}
}

func TestStartupBannerPrefersActiveWorkflowOverUnsettledPlan(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	if _, err := planStore.SaveForTest(&types.ChangePlan{
		ID:      "plan-banner-approval",
		Status:  types.PlanStatusPending,
		Summary: "high risk workflow approval",
	}); err != nil {
		t.Fatalf("SaveForTest: %v", err)
	}
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	run := types.WriteWorkflowRun{
		RunID:         "wf-banner-approval",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-approval",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-approval",
			Status: types.WriteWorkflowBatchPendingApproval,
			PlanID: "plan-banner-approval",
		}},
	}
	if _, err := workflowStore.Save(&run); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	line := writeWorkflowBannerLine("en", run, types.DeriveWriteWorkflowNextActionView(run))
	for _, want := range []string{"write workflow `wf-banner-approval` needs approval", "batch `batch-approval`", "plan `plan-banner-approval`"} {
		if !strings.Contains(line, want) {
			t.Fatalf("workflow banner helper missing %q: %q", want, line)
		}
	}
	out := &bytes.Buffer{}
	r := &REPL{
		out:                   out,
		headerPrinted:         true,
		language:              "en",
		planStore:             planStore,
		writeWorkflowRunStore: workflowStore,
	}
	r.banner()
	got := out.String()
	for _, want := range []string{"write workflow `wf-banner-approval` needs approval", "/workflow show audits fingerprint/diff"} {
		if !strings.Contains(got, want) {
			t.Fatalf("startup banner missing workflow state %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Auto Pilot paused: an unsettled change exists") {
		t.Fatalf("startup banner should prefer workflow state over legacy unsettled plan text:\n%s", got)
	}
}

func TestWorkflowClearDeletesActiveWriteWorkflow(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-clear",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
		ContextPacks: []types.WriteContextPack{{
			PackID:  "pack-1",
			BatchID: "batch-1",
			Items: []types.WriteContextItem{{
				Priority: types.WriteContextP1,
				Kind:     "evidence_ref",
				Text:     "internal/repl/repl.go:1",
			}},
		}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                stubRunner{},
		In:                    strings.NewReader(""),
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
	})

	r.handleWorkflowCmd("/workflow clear")

	loaded, err := workflowStore.Load("wf-clear")
	if err != nil {
		t.Fatalf("Load after clear: %v", err)
	}
	if loaded != nil {
		t.Fatal("workflow should be deleted after /workflow clear")
	}
}
