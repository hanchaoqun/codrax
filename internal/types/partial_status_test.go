package types

import "testing"

// TestPlanStatus_PartiallyApplied verifies the new enum value is
// recognised by IsUnsettledStatus (so the single-pending-plan
// invariant treats it correctly — a partial state still blocks
// new plans in the same project).
func TestPlanStatus_PartiallyApplied(t *testing.T) {
	if !IsUnsettledStatus(PlanStatusPartiallyApplied) {
		t.Errorf("PlanStatusPartiallyApplied (%q) must be unsettled — partial state blocks new plans", PlanStatusPartiallyApplied)
	}
}

func TestPlanStatus_AppliedPendingVerifyIsUnsettled(t *testing.T) {
	if !IsUnsettledStatus(PlanStatusAppliedPendingVerify) {
		t.Errorf("PlanStatusAppliedPendingVerify (%q) must be unsettled — verify still has to settle the plan", PlanStatusAppliedPendingVerify)
	}
}

// TestPlanStatus_TerminalsStaySettled pins the failure-vs-partial
// distinction: applied_failed (apply produced zero successful units;
// worktree clean) is settled / terminal, while partially_applied
// (apply succeeded on N units before failing) is unsettled and
// requires operator action.
func TestPlanStatus_TerminalsStaySettled(t *testing.T) {
	if IsUnsettledStatus(PlanStatusApplyFailed) {
		t.Error("PlanStatusApplyFailed must remain settled (terminal) — distinct from partially_applied")
	}
	if IsUnsettledStatus(PlanStatusMerged) {
		t.Error("PlanStatusMerged must remain settled (terminal)")
	}
	if IsUnsettledStatus(PlanStatusRejected) {
		t.Error("PlanStatusRejected must remain settled (terminal)")
	}
	if IsUnsettledStatus(PlanStatusBlocked) {
		t.Error("PlanStatusBlocked must remain settled (terminal)")
	}
	if IsUnsettledStatus(PlanStatusNoChangeRequired) {
		t.Error("PlanStatusNoChangeRequired must remain an internal settled sentinel, not a user-visible active plan")
	}
}
