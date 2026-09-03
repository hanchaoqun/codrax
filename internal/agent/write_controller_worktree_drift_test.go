package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_controller_worktree_drift_test.go — V5-2 (§40.36): the controller's
// effect lines carry the typed drift class and disposition so the planner
// is never asked to "fix" a disclosed lockfile.
func TestWriteControllerPromptCarriesDriftClassAndDisposition(t *testing.T) {
	mut := types.NewMutableState("verify with disclosed drift")
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-drift", Status: types.PlanStatusApplied})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID: "plan-drift", Passed: true, VerificationStatus: types.VerificationStatusPassed,
		WorktreeAudit: &types.VerificationWorktreeAudit{
			Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, ReasonCode: types.VerificationTrackedSideEffectDisclosedReason,
			TrackedEffectCount: 1, DisclosedTrackedEffectCount: 1,
			Effects: []types.VerificationWorktreeEffect{{
				Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
				Action: "disclosed_not_committed_not_auto_reverted", DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
				OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed,
			}},
		},
	})
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	for _, want := range []string{
		"verification_worktree_audit: status=tracked_drift_disclosed",
		"verification_worktree_effect: path=Cargo.lock kind=tracked_changed ownership=git_tracked action=disclosed_not_committed_not_auto_reverted drift_class=dependency_lockfile_refresh disposition=disclosed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt lost %q:\n%s", want, got)
		}
	}
}
