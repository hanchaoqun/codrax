package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDeriveApprovalExecutionViewMissingApproval(t *testing.T) {
	view := DeriveApprovalExecutionView(&types.ChangePlan{ID: "plan-missing-approval"})
	if view.State != ApprovalExecutionMissing {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionMissing, view)
	}
	if view.CanApply || view.CanAutoApply || view.RequiresUser || view.Denied {
		t.Fatalf("missing approval should be inert: %+v", view)
	}
}

func TestDeriveApprovalExecutionViewAutoAllowed(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")

	view := DeriveApprovalExecutionView(plan)
	if view.State != ApprovalExecutionAutoAllowed {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionAutoAllowed, view)
	}
	if !view.CanApply || !view.CanAutoApply || view.RequiresUser || view.Denied {
		t.Fatalf("auto approval flags wrong: %+v", view)
	}
}

func TestDeriveApprovalExecutionViewManualRequired(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionManual, "required")

	view := DeriveApprovalExecutionView(plan)
	if view.State != ApprovalExecutionManualRequired {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionManualRequired, view)
	}
	if !view.RequiresUser || view.CanApply || view.CanAutoApply {
		t.Fatalf("manual-required flags wrong: %+v", view)
	}
}

func TestDeriveApprovalExecutionViewManualApproved(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionManual, "approved")

	view := DeriveApprovalExecutionView(plan)
	if view.State != ApprovalExecutionManualApproved {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionManualApproved, view)
	}
	if !view.CanApply || view.CanAutoApply || view.RequiresUser {
		t.Fatalf("manual-approved flags wrong: %+v", view)
	}
}

func TestDeriveApprovalExecutionViewDenied(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionDeny, "denied")

	view := DeriveApprovalExecutionView(plan)
	if view.State != ApprovalExecutionDenied {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionDenied, view)
	}
	if !view.Denied || view.CanApply || view.RequiresUser {
		t.Fatalf("deny flags wrong: %+v", view)
	}
}

func TestDeriveApprovalExecutionViewRejectsStaleFingerprint(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	plan.Summary = "changed after approval"

	view := DeriveApprovalExecutionView(plan)
	if view.State != ApprovalExecutionFingerprintMismatch {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionFingerprintMismatch, view)
	}
	if !view.RequiresUser || view.CanApply || view.CanAutoApply {
		t.Fatalf("stale approval flags wrong: %+v", view)
	}
}

func TestDeriveApprovalExecutionViewRejectsTamperedRecord(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalActionAutoExecute, "auto")
	plan.Approval.RecordFingerprint = "not-the-record-fingerprint"

	view := DeriveApprovalExecutionView(plan)
	if view.State != ApprovalExecutionIntegrityFailed {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionIntegrityFailed, view)
	}
	if !view.RequiresUser || view.CanApply || view.CanAutoApply {
		t.Fatalf("tampered approval flags wrong: %+v", view)
	}
}

func TestDeriveApprovalExecutionViewUnknownActionRequiresUser(t *testing.T) {
	plan := approvalExecutionPlanForTest(ApprovalAction("surprise"), "")

	view := DeriveApprovalExecutionView(plan)
	if view.State != ApprovalExecutionUnknownAction {
		t.Fatalf("state = %q, want %q: %+v", view.State, ApprovalExecutionUnknownAction, view)
	}
	if !view.RequiresUser || view.CanApply {
		t.Fatalf("unknown action flags wrong: %+v", view)
	}
}

func approvalExecutionPlanForTest(action ApprovalAction, userDecision string) *types.ChangePlan {
	plan := &types.ChangePlan{
		ID:          "plan-approval-execution",
		Summary:     "repair approval authority",
		TargetPaths: []string{"pkg/fix.py"},
		Changes: []types.FileChange{{
			Path:       "pkg/fix.py",
			Kind:       "modify",
			NewContent: "value = 1\n",
		}},
		Approval: &types.WriteApprovalRecord{
			Action:       string(action),
			UserDecision: userDecision,
			ReasonCode:   "test_reason",
		},
	}
	plan.Approval.PlanFingerprint = types.PlanFingerprint(plan)
	return plan
}
