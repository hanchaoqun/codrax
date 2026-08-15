package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func TestCaptureReplanCurrentWorktreeReceiptBindsAppliedGenerationAndCurrentBytes(t *testing.T) {
	root := t.TempDir()
	current := "class relativedelta(object):\n    def __init__(self, years=0):\n        self.years = self._normalize(years)\n\n    def _normalize(self, value):\n        return int(value)\n"
	if err := os.WriteFile(filepath.Join(root, "relativedelta.py"), []byte(current), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mu := types.NewMutableState("replan current state")
	plan := &types.ChangePlan{
		ID:           "plan-current",
		WorktreePath: root,
		Changes: []types.FileChange{{
			Path:  "relativedelta.py",
			Kind:  "patch",
			Apply: &types.FileChangeApplyRecord{Status: "applied"},
		}},
		PatchEffect: &types.PatchEffectRecord{
			RecordID:        "patch-effect:plan-current",
			PlanID:          "plan-current",
			DiffFingerprint: "diff-sha",
			Files: []types.PatchEffectFile{{
				Path: "relativedelta.py",
				Hunks: []types.PatchEffectHunk{{
					NewStart:         2,
					AddedLineNumbers: []int{3, 4, 5, 6},
					AddedLineTexts: []types.PatchEffectLine{
						{Line: 3, Text: "        self.years = self._normalize(years)"},
						{Line: 4, Text: ""},
						{Line: 5, Text: "    def _normalize(self, value):"},
						{Line: 6, Text: "        return int(value)"},
					},
					RemovedLineTexts: []types.PatchEffectLine{{Line: 3, Text: "        self.years = years"}},
				}},
			}},
		},
	}
	mu.SetChangePlan(plan)
	run := &types.WriteWorkflowRun{
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1",
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: "plan-earlier"},
				{Kind: "apply", Status: "applied", PlanID: "plan-current"},
			},
		}},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorktreePath: root, Mode: types.ModeApply}}
	o.captureReplanCurrentWorktreeReceipt(run, plan, "truth_ledger_failed_requires_repair")

	receipt := mu.ReplanCurrentWorktreeReceipt()
	if receipt == nil {
		t.Fatal("expected current-worktree receipt")
	}
	if receipt.BatchID != "batch-1" || receipt.SourcePlanID != "plan-current" || receipt.ApplyGeneration != 2 ||
		receipt.DiffFingerprint != "diff-sha" || receipt.TriggerReasonCode != "truth_ledger_failed_requires_repair" {
		t.Fatalf("unexpected receipt identity: %+v", receipt)
	}
	if len(receipt.Paths) != 1 {
		t.Fatalf("paths=%d, want 1: %+v", len(receipt.Paths), receipt.Paths)
	}
	path := receipt.Paths[0]
	if path.State != types.ReplanWorktreePathPresent || path.CurrentBytes != len(current) || len(path.CurrentSHA256) != 64 {
		t.Fatalf("unexpected current path authority: %+v", path)
	}
	if !path.AppliedEditComplete || path.AppliedEditTotal != 5 || len(path.AppliedEdits) != 5 {
		t.Fatalf("exact applied edit receipt incomplete: %+v", path)
	}
	if len(path.CurrentSourceSnapshots) != 1 || !strings.Contains(path.CurrentSourceSnapshots[0].Snippet, "def _normalize") {
		t.Fatalf("current source snapshot missing already-applied implementation: %+v", path.CurrentSourceSnapshots)
	}

	o.prepareControllerPlanningState()
	if mu.ChangePlan() != nil {
		t.Fatal("planning reset should clear ChangePlan")
	}
	if mu.ReplanCurrentWorktreeReceipt() == nil {
		t.Fatal("planning reset must preserve the current-worktree receipt")
	}
}

func TestCaptureReplanCurrentWorktreeReceiptRejectsMismatchedPatchEffectPlan(t *testing.T) {
	mu := types.NewMutableState("mismatched patch effect")
	mu.SetReplanCurrentWorktreeReceipt(&types.ReplanCurrentWorktreeReceipt{SourcePlanID: "stale"})
	plan := &types.ChangePlan{
		ID:           "plan-current",
		WorktreePath: t.TempDir(),
		Changes:      []types.FileChange{{Path: "x.go", Apply: &types.FileChangeApplyRecord{Status: "applied"}}},
		PatchEffect:  &types.PatchEffectRecord{PlanID: "plan-other", Files: []types.PatchEffectFile{{Path: "x.go"}}},
	}
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorktreePath: plan.WorktreePath, Mode: types.ModeApply}}
	o.captureReplanCurrentWorktreeReceipt(&types.WriteWorkflowRun{ActiveBatchID: "batch-1"}, plan, "replan")
	if got := mu.ReplanCurrentWorktreeReceipt(); got != nil {
		t.Fatalf("mismatched effect authority must not survive, got %+v", got)
	}
}

func TestTruthLedgerReplanFromNominalPassInstallsCurrentWorktreeReceipt(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fix.py"), []byte("def fixed():\n    return True\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mu := types.NewMutableState("nominal pass with failed typed truth")
	plan := &types.ChangePlan{
		ID: "plan-nominal-pass", WorktreePath: root,
		Changes: []types.FileChange{{Path: "fix.py", Kind: "patch", Apply: &types.FileChangeApplyRecord{Status: "applied"}}},
		PatchEffect: &types.PatchEffectRecord{
			PlanID: "plan-nominal-pass", DiffFingerprint: "diff-fingerprint",
			Files: []types.PatchEffectFile{{Path: "fix.py", Hunks: []types.PatchEffectHunk{{
				NewStart: 1, AddedLineNumbers: []int{1},
				AddedLineTexts: []types.PatchEffectLine{{Line: 1, Text: "def fixed():"}},
			}}}},
		},
		// A typed hard patch finding is used here to exercise the generic
		// nominal-report-pass truth override. The production r506 trigger was a
		// failed cumulative proof obligation; both enter the same truth replan
		// producer and neither creates VerifyFailureHandoff.
		PatchReview: &types.PatchReviewRecord{HardBlock: true, Findings: []types.PatchReviewFinding{{
			Code: "typed_patch_truth_failed", Severity: types.PatchReviewSeverityError, Path: "fix.py",
		}}},
	}
	report := &types.ChangeReport{PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: true, VerificationStatus: types.VerificationStatusPassed}
	mu.SetChangePlan(plan)
	mu.SetChangeReport(report)
	run := &types.WriteWorkflowRun{
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID: "batch-1", PlanID: plan.ID, Status: types.WriteWorkflowBatchComplete,
			Attempts: []types.WriteWorkflowAttempt{
				{Kind: "apply", Status: "applied", PlanID: plan.ID},
				{Kind: "verify", Status: "passed", PlanID: plan.ID},
			},
		}},
	}
	mu.SetWriteWorkflowRun(run)
	o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorktreePath: root, Mode: types.ModeApply}}

	decision := o.normalizeControllerTypedStateDecision(writeflow.WriteWorkflowDecision{Action: writeflow.ActionFinish}, run)
	if decision.Action != writeflow.ActionReplanBatch || decision.ReasonCode != "truth_ledger_failed_requires_repair" {
		t.Fatalf("nominal report pass with failed typed truth must replan, got %+v", decision)
	}
	if mu.VerifyFailureHandoff() != nil {
		t.Fatal("nominally passing report must not manufacture a VerifyFailureHandoff")
	}
	receipt := mu.ReplanCurrentWorktreeReceipt()
	if receipt == nil || receipt.SourcePlanID != plan.ID || receipt.TriggerReasonCode != "truth_ledger_failed_requires_repair" || len(receipt.Paths) != 1 {
		t.Fatalf("truth replan producer did not install current-state receipt: %+v", receipt)
	}
}

func TestReplanAppliedEditReceiptBoundsDisplayButKeepsExactIdentity(t *testing.T) {
	raw := strings.Repeat("界", 400)
	receipt := newReplanAppliedEditReceipt("added", 7, raw)
	if receipt.TextComplete || receipt.TextBytes != len([]byte(raw)) || len(receipt.TextSHA256) != 64 {
		t.Fatalf("long receipt must retain exact byte/hash identity with bounded text: %+v", receipt)
	}
	if len([]byte(receipt.Text)) > maxReplanAppliedEditTextBytes {
		t.Fatalf("display text exceeded bound: bytes=%d bound=%d", len([]byte(receipt.Text)), maxReplanAppliedEditTextBytes)
	}
}
