package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_controller_worktree_fold_in_test.go — F-run-tests fold-in of V5-2
// (§40.36 复核收编 F3): the controller's effect line carries the typed
// lockfile fixed-point state and, when unproven, its plain-words disclosure,
// so the controller never treats a cut-short suite's lockfile as proven.
func TestWriteControllerPromptCarriesLockfileFixedPoint(t *testing.T) {
	mut := types.NewMutableState("verify with unproven lockfile fixed point")
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
				LockfileFixedPoint: types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded,
			}},
		},
	})
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	for _, want := range []string{
		"drift_class=dependency_lockfile_refresh disposition=disclosed lockfile_fixed_point=unproven_suite_infra_downgraded\n",
		"- verification_lockfile_fixed_point: " + types.WriteContextLockfileFixedPointDisclosureText("Cargo.lock", types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded) + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt lost %q:\n%s", want, got)
		}
	}
}
