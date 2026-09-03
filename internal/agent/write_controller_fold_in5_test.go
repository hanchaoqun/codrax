package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_controller_fold_in5_test.go — fold-in round five
// (colleague_merge_audit §40.36 五轮收编, finding II): the controller prompt
// caps the worktree effect list at 8 rows, and before this fix the
// verification_lockfile_fixed_point disclosure line was emitted only inside
// that capped loop — with 8 refused rows ahead of the lockfile row the
// prompt carried no lockfile_fixed_point token at all. Typed disclosures are
// must-carry: the disclosure line is emitted OUTSIDE the cap (always, once
// per lockfile row), and the capped effect list orders lockfile-class rows
// first (disclosed lockfile rows before refused rows), so the row that
// carries the token is never the one the cap trims.
func TestWriteControllerPromptCarriesLockfileDisclosureBehindEightRefusedRows(t *testing.T) {
	lockfile := types.VerificationWorktreeEffect{
		Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
		Action: "disclosed_not_committed_not_auto_reverted", DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
		OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed,
		LockfileFixedPoint: types.VerificationLockfileFixedPointUnprovenRunRefused,
	}
	var effects []types.VerificationWorktreeEffect
	for i := 0; i < 8; i++ {
		effects = append(effects, types.VerificationWorktreeEffect{
			Path: "src/refused_" + string(rune('a'+i)) + ".rs", Kind: types.VerificationWorktreeEffectTrackedChanged,
			Ownership: "git_tracked", Action: "verification_failed_retained_for_review",
			DriftClass: types.VerificationWorktreeDriftUnclassified, Disposition: types.VerificationWorktreeEffectRefused,
		})
	}
	effects = append(effects, lockfile) // the 9th row: past the cap
	mut := types.NewMutableState("mixed refused run, lockfile last")
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-ii", Status: types.PlanStatusVerifyFailed})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID: "plan-ii", Passed: false, FailureKind: types.FailureKindVerificationSideEffect,
		WorktreeAudit: &types.VerificationWorktreeAudit{
			Status: types.VerificationWorktreeAuditTrackedDrift, TrackedEffectCount: 9,
			DisclosedTrackedEffectCount: 1, RefusedTrackedEffectCount: 8,
			Effects: effects,
		},
	})
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	// The must-carry disclosure line is present, once, outside the cap.
	disclosure := "- verification_lockfile_fixed_point: " + types.WriteContextLockfileFixedPointDisclosureText("Cargo.lock", types.VerificationLockfileFixedPointUnprovenRunRefused) + "\n"
	if strings.Count(got, disclosure) != 1 {
		t.Fatalf("the prompt must carry the lockfile fixed-point disclosure exactly once past the 8-row cap:\n%s", got)
	}
	// The lockfile row itself is ordered first in the capped effect list, so
	// its typed tokens survive too.
	var effectLines []string
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "- verification_worktree_effect: ") {
			effectLines = append(effectLines, line)
		}
	}
	if len(effectLines) != 9 || !strings.HasPrefix(effectLines[8], "- verification_worktree_effect: ... +1 more") {
		t.Fatalf("the 8-row cap must still apply (8 rows + overflow marker), got %d lines: %v", len(effectLines), effectLines)
	}
	if !strings.Contains(effectLines[0], "lockfile_fixed_point=unproven_run_refused") || !strings.Contains(effectLines[0], "path=Cargo.lock") {
		t.Fatalf("the disclosed lockfile row must be listed first, before the refused rows: %q", effectLines[0])
	}
	if strings.Count(got, "lockfile_fixed_point=") < 2 {
		t.Fatalf("the prompt lost the lockfile_fixed_point token:\n%s", got)
	}
}

// A lockfile row with a proven fixed point emits no disclosure line (the
// phrase is empty), and refused-only audits emit none at all — the
// must-carry loop mints nothing new.
func TestWriteControllerPromptEmitsNoDisclosureLineWithoutAnUnprovenLockfileRow(t *testing.T) {
	mut := types.NewMutableState("proven lockfile")
	mut.SetChangePlan(&types.ChangePlan{ID: "plan-ok", Status: types.PlanStatusApplied})
	mut.SetChangeReport(&types.ChangeReport{
		PlanID: "plan-ok", Passed: true,
		WorktreeAudit: &types.VerificationWorktreeAudit{
			Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, TrackedEffectCount: 1, DisclosedTrackedEffectCount: 1,
			Effects: []types.VerificationWorktreeEffect{{
				Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
				Action: "disclosed_not_committed_not_auto_reverted", DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
				OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed,
				LockfileFixedPoint: types.VerificationLockfileFixedPointProven,
			}},
		},
	})
	got := (&writeControllerEvaluator{}).BuildInitialInstruction(&types.AgentContext{Mutable: mut}, nil)
	if strings.Contains(got, "- verification_lockfile_fixed_point: ") {
		t.Fatalf("a proven fixed point has no disclosure phrase, so no line:\n%s", got)
	}
	if !strings.Contains(got, "lockfile_fixed_point=proven") {
		t.Fatalf("the effect row still carries the typed token:\n%s", got)
	}
}
