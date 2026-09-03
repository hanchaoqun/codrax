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
//
// EVOLUTION RECORD (§40.36 三轮收编, finding E): the effect line now shares
// types.WriteContextWorktreeEffectText with the context pack — typed tokens
// first, the path LAST — so the pinned line moved `path=` to the end; the
// disclosure line likewise ends with the path.
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
		"- verification_worktree_effect: kind=tracked_changed ownership=git_tracked action=disclosed_not_committed_not_auto_reverted drift_class=dependency_lockfile_refresh disposition=disclosed owner_runner=rust lockfile_fixed_point=unproven_suite_infra_downgraded path=Cargo.lock\n",
		"- verification_lockfile_fixed_point: " + types.WriteContextLockfileFixedPointDisclosureText("Cargo.lock", types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded) + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("controller prompt lost %q:\n%s", want, got)
		}
	}
}

// E (controller line) + A (run-refused phrase): for long and deep lockfile
// paths the controller's effect line keeps every typed token and the whole
// plain-words phrase, with the path as the last element; a mixed refused
// run's lockfile row is never shown without its unproven_run_refused state.
func TestWriteControllerPromptKeepsTypedTokensForLongLockfilePaths(t *testing.T) {
	paths := []string{"Cargo.lock", "pnpm-lock.yaml", "package-lock.json", "crates/foo/Cargo.lock", "services/backend/packages/api-gateway/package-lock.json"}
	tokens := []string{"kind=tracked_changed", "ownership=git_tracked", "action=disclosed_not_committed_not_auto_reverted",
		"drift_class=dependency_lockfile_refresh", "disposition=disclosed", "owner_runner=rust", "lockfile_fixed_point=unproven_run_refused"}
	phrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenRunRefused, false)
	for _, path := range paths {
		effect := types.VerificationWorktreeEffect{
			Path: path, Kind: types.VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
			Action: "disclosed_not_committed_not_auto_reverted", DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
			OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed, LockfileFixedPoint: types.VerificationLockfileFixedPointUnprovenRunRefused,
		}
		mut := types.NewMutableState("mixed refused run")
		mut.SetChangePlan(&types.ChangePlan{ID: "plan-mixed", Status: types.PlanStatusVerifyFailed})
		mut.SetChangeReport(&types.ChangeReport{
			PlanID: "plan-mixed", Passed: false, FailureKind: types.FailureKindVerificationSideEffect,
			WorktreeAudit: &types.VerificationWorktreeAudit{
				Status: types.VerificationWorktreeAuditTrackedDrift, TrackedEffectCount: 2, DisclosedTrackedEffectCount: 1, RefusedTrackedEffectCount: 1,
				Effects: []types.VerificationWorktreeEffect{effect,
					{Path: "src/other.rs", Kind: types.VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked", Action: "verification_failed_retained_for_review",
						DriftClass: types.VerificationWorktreeDriftUnclassified, Disposition: types.VerificationWorktreeEffectRefused}},
			},
		})
		got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
		line := ""
		for _, candidate := range strings.Split(got, "\n") {
			if strings.HasPrefix(candidate, "- verification_worktree_effect: ") && strings.Contains(candidate, "dependency_lockfile_refresh") {
				line = candidate
			}
		}
		if line == "" {
			t.Fatalf("%s: lockfile effect line missing:\n%s", path, got)
		}
		for _, token := range tokens {
			if !strings.Contains(line, " "+token+" ") && !strings.Contains(line, ": "+token+" ") {
				t.Fatalf("%s: controller line lost %q: %q", path, token, line)
			}
		}
		// The path is the last element: whole when it fits the shared item
		// bound, otherwise "…" plus a tail of the path — never a cut token.
		seg := line[strings.LastIndex(line, " path=")+len(" path="):]
		if tail := strings.TrimPrefix(seg, "…"); !strings.Contains(line, " path=") || !(seg == path || (tail != seg && tail != "" && strings.HasSuffix(path, tail))) {
			t.Fatalf("%s: the path must be the last element: %q", path, line)
		}
		disclosure := "- verification_lockfile_fixed_point: " + types.WriteContextLockfileFixedPointDisclosureText(path, types.VerificationLockfileFixedPointUnprovenRunRefused) + "\n"
		if !strings.Contains(got, disclosure) || !strings.Contains(disclosure, phrase) || !strings.Contains(disclosure, ") path=") {
			t.Fatalf("%s: controller prompt lost the run-refused disclosure line %q:\n%s", path, disclosure, got)
		}
	}
}
