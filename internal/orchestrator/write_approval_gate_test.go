package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func writeApprovalGateFixture(plan *types.ChangePlan, mode types.PipelineMode, policy writeflow.ApprovalPolicy) *Orchestrator {
	mu := types.NewMutableState("write approval gate")
	mu.SetChangePlan(plan)
	return &Orchestrator{
		writeApprovalPolicy: policy,
		busCtx: &types.BusContext{
			Mode:       mode,
			Language:   "en",
			Mutable:    mu,
			AnalysisIR: &types.AnalysisIR{},
		},
	}
}

func TestPlanPostHook_WriteApprovalAutoSafeAllowsMediumBeforeApply(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-medium",
		Summary:     "modify source",
		TargetPaths: []string{"internal/foo.go"},
		Changes:     []types.FileChange{{Path: "internal/foo.go", Kind: "modify", Rationale: "test"}},
	}
	o := writeApprovalGateFixture(plan, types.ModeApply, writeflow.ApprovalPolicyAutoSafe)

	if err := planPostHook(o, nil); err != nil {
		t.Fatalf("auto_safe should allow medium-risk source changes before apply: %v", err)
	}
	if plan.Approval == nil || plan.Approval.Action != string(writeflow.ApprovalActionAutoExecute) {
		t.Fatalf("auto-safe plan should receive approval record, got %+v", plan.Approval)
	}
}

func TestPlanPostHook_WriteApprovalManualBlocksFreshApplyPlan(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-doc",
		Summary:     "edit docs",
		TargetPaths: []string{"docs/guide.md"},
		Changes:     []types.FileChange{{Path: "docs/guide.md", Kind: "modify", Rationale: "test"}},
	}
	o := writeApprovalGateFixture(plan, types.ModeApply, writeflow.ApprovalPolicyManual)

	err := planPostHook(o, nil)
	if err == nil {
		t.Fatal("manual write policy should pause a freshly generated apply plan")
	}
	if !strings.Contains(err.Error(), "write approval required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := o.busCtx.Mutable.Result(); !strings.Contains(got, "manual_approval") {
		t.Fatalf("result should explain manual approval, got: %q", got)
	}
	if plan.Approval == nil || plan.Approval.UserDecision != "required" {
		t.Fatalf("manual pause should record required approval, got %+v", plan.Approval)
	}
}

func TestPlanPostHook_WriteApprovalAutoSafeDeniesCriticalBeforeApply(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-critical",
		Summary:     "outside path",
		TargetPaths: []string{"../outside.go"},
		Changes:     []types.FileChange{{Path: "../outside.go", Kind: "modify", Rationale: "test"}},
	}
	o := writeApprovalGateFixture(plan, types.ModeApply, writeflow.ApprovalPolicyAutoSafe)

	err := planPostHook(o, nil)
	if err == nil {
		t.Fatal("critical write risk should be denied before apply")
	}
	if !strings.Contains(err.Error(), "write approval denied") {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := o.busCtx.Mutable.Result(); !strings.Contains(got, "outside_repo_path") {
		t.Fatalf("result should explain critical path risk, got: %q", got)
	}
	if plan.Status != types.PlanStatusBlocked {
		t.Fatalf("critical denial status = %q, want %q", plan.Status, types.PlanStatusBlocked)
	}
}

func TestPlanPostHook_WriteApprovalDoesNotBlockPlanMode(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-preview",
		Summary:     "edit docs",
		TargetPaths: []string{"docs/guide.md"},
		Changes:     []types.FileChange{{Path: "docs/guide.md", Kind: "modify", Rationale: "test"}},
	}
	o := writeApprovalGateFixture(plan, types.ModePlan, writeflow.ApprovalPolicyManual)

	if err := planPostHook(o, nil); err != nil {
		t.Fatalf("plan mode should produce a reviewable ChangePlan without approval gate: %v", err)
	}
}

func TestApplyPreHook_PlanFileAutoSafeRecordsApprovalBeforeWorktree(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-file-auto",
		Status:      types.PlanStatusPending,
		Summary:     "edit docs",
		TargetPaths: []string{"docs/guide.md"},
		Changes:     []types.FileChange{{Path: "docs/guide.md", Kind: "modify", Rationale: "test"}},
	}
	path := writeApprovalTempPlan(t, plan)
	o := writeApprovalApplyPreFixture(path, writeflow.ApprovalPolicyAutoSafe)

	err := applyPreHook(o)
	if err == nil {
		t.Fatal("expected applyPreHook to continue past approval and fail on missing worktree base")
	}
	if strings.Contains(err.Error(), "write approval") {
		t.Fatalf("auto-safe plan-file apply should not fail at approval gate: %v", err)
	}
	reloaded, loadErr := types.LoadChangePlanFromFile(path)
	if loadErr != nil {
		t.Fatalf("reload plan: %v", loadErr)
	}
	if reloaded.Approval == nil || reloaded.Approval.Action != string(writeflow.ApprovalActionAutoExecute) {
		t.Fatalf("approval record not persisted: %+v", reloaded.Approval)
	}
}

func TestApplyPreHook_PlanFileManualBlocksBeforeWorktree(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-file-manual",
		Status:      types.PlanStatusPending,
		Summary:     "edit docs",
		TargetPaths: []string{"docs/guide.md"},
		Changes:     []types.FileChange{{Path: "docs/guide.md", Kind: "modify", Rationale: "test"}},
	}
	path := writeApprovalTempPlan(t, plan)
	o := writeApprovalApplyPreFixture(path, writeflow.ApprovalPolicyManual)

	err := applyPreHook(o)
	if err == nil || !strings.Contains(err.Error(), "write approval required") {
		t.Fatalf("manual plan-file apply should pause at approval gate, got %v", err)
	}
	if strings.Contains(err.Error(), "worktree base") {
		t.Fatalf("approval gate must run before worktree provisioning, got %v", err)
	}
	reloaded, loadErr := types.LoadChangePlanFromFile(path)
	if loadErr != nil {
		t.Fatalf("reload plan: %v", loadErr)
	}
	if reloaded.Approval == nil || reloaded.Approval.UserDecision != "required" {
		t.Fatalf("manual approval record not persisted: %+v", reloaded.Approval)
	}
}

func TestApplyPreHook_PlanFileApprovedManualPassesFingerprintGate(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-file-approved",
		Status:      types.PlanStatusPending,
		Summary:     "edit docs",
		TargetPaths: []string{"docs/guide.md"},
		Changes:     []types.FileChange{{Path: "docs/guide.md", Kind: "modify", Rationale: "test"}},
	}
	assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
	decision := writeflow.DecideWriteApproval(writeflow.ApprovalPolicyManual, assessment)
	plan.Approval = writeflow.NewApprovalRecord(assessment, decision, "test", "approved", types.PlanFingerprint(plan), "")
	path := writeApprovalTempPlan(t, plan)
	o := writeApprovalApplyPreFixture(path, writeflow.ApprovalPolicyManual)

	err := applyPreHook(o)
	if err == nil {
		t.Fatal("expected applyPreHook to continue past approval and fail on missing worktree base")
	}
	if strings.Contains(err.Error(), "write approval") {
		t.Fatalf("approved manual plan-file apply should not fail at approval gate: %v", err)
	}
}

func TestApplyPreHook_PlanFileStaleManualApprovalBlocks(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-file-stale",
		Status:      types.PlanStatusPending,
		Summary:     "edit docs",
		TargetPaths: []string{"docs/guide.md"},
		Changes:     []types.FileChange{{Path: "docs/guide.md", Kind: "modify", Rationale: "test"}},
	}
	assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
	decision := writeflow.DecideWriteApproval(writeflow.ApprovalPolicyManual, assessment)
	plan.Approval = writeflow.NewApprovalRecord(assessment, decision, "test", "approved", "stale-fingerprint", "")
	path := writeApprovalTempPlan(t, plan)
	o := writeApprovalApplyPreFixture(path, writeflow.ApprovalPolicyManual)

	err := applyPreHook(o)
	if err == nil || !strings.Contains(err.Error(), "stale_approval_fingerprint") {
		t.Fatalf("stale manual approval should block, got %v", err)
	}
}

func TestApplyPreHook_PlanFileCriticalDeniedBeforeWorktree(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-file-critical",
		Status:      types.PlanStatusPending,
		Summary:     "outside path",
		TargetPaths: []string{"../outside.go"},
		Changes:     []types.FileChange{{Path: "../outside.go", Kind: "modify", Rationale: "test"}},
	}
	path := writeApprovalTempPlan(t, plan)
	o := writeApprovalApplyPreFixture(path, writeflow.ApprovalPolicyAutoSafe)

	err := applyPreHook(o)
	if err == nil || !strings.Contains(err.Error(), "write approval denied") {
		t.Fatalf("critical plan-file apply should be denied, got %v", err)
	}
	if strings.Contains(err.Error(), "worktree base") {
		t.Fatalf("critical denial must run before worktree provisioning, got %v", err)
	}
	reloaded, loadErr := types.LoadChangePlanFromFile(path)
	if loadErr != nil {
		t.Fatalf("reload plan: %v", loadErr)
	}
	if reloaded.Status != types.PlanStatusBlocked {
		t.Fatalf("denied plan status = %q, want %q", reloaded.Status, types.PlanStatusBlocked)
	}
	if reloaded.Approval == nil || reloaded.Approval.Action != string(writeflow.ApprovalActionDeny) {
		t.Fatalf("deny approval record not persisted: %+v", reloaded.Approval)
	}
}

func writeApprovalApplyPreFixture(planPath string, policy writeflow.ApprovalPolicy) *Orchestrator {
	return &Orchestrator{
		writeApprovalPolicy: policy,
		busCtx: &types.BusContext{
			Mode:       types.ModeApply,
			Language:   "en",
			PlanPath:   planPath,
			Mutable:    types.NewMutableState("write approval apply pre"),
			AnalysisIR: &types.AnalysisIR{},
		},
	}
}

func writeApprovalTempPlan(t *testing.T, plan *types.ChangePlan) string {
	t.Helper()
	path := t.TempDir() + "/plan.json"
	if err := types.WritePlanToFile(plan, path); err != nil {
		t.Fatalf("write temp plan: %v", err)
	}
	return path
}
