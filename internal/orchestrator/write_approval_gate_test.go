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
