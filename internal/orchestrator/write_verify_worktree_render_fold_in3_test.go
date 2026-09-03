package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// write_verify_worktree_render_fold_in3_test.go — F-run-tests round-three
// fold-in of V5-2 (§40.36 三轮收编, finding F): the worktree-audit note is
// rendered on ALL THREE verify outcomes from the one shared predicate. The
// F6 fix of the previous round only reached renderVerifySuccess, which a
// refused run (status failed → verifier error → renderVerifyFailure) and an
// unavailable verdict (renderVerifyUnverified) never call, so the REPL
// verify surface for a refused run still hid its untracked outputs and its
// disclosed lockfile rows. Pinned here at the stage-hook level.

func seedVerifyPlan(t *testing.T, id string) (*types.ChangePlan, string) {
	t.Helper()
	plan := &types.ChangePlan{
		ID: id, Status: types.PlanStatusAppliedPendingVerify, Summary: "fix", Request: "fix",
		TargetPaths: []string{"src/lib.rs"},
		Changes:     []types.FileChange{{Path: "src/lib.rs", Kind: "modify", NewContent: "pub fn a() {}\n"}},
	}
	planPath := t.TempDir() + "/" + id + ".json"
	if err := types.WritePlanToFile(plan, planPath); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	return plan, planPath
}

func refusedRunAudit() *types.VerificationWorktreeAudit {
	return &types.VerificationWorktreeAudit{
		Status: types.VerificationWorktreeAuditTrackedDrift, ReasonCode: "verification_tracked_worktree_drift",
		TrackedEffectCount: 2, UntrackedEffectCount: 1, DisclosedTrackedEffectCount: 1, RefusedTrackedEffectCount: 1,
		Effects: []types.VerificationWorktreeEffect{
			{Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
				OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed, LockfileFixedPoint: types.VerificationLockfileFixedPointUnprovenRunRefused},
			{Path: "src/other.rs", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftUnclassified, Disposition: types.VerificationWorktreeEffectRefused},
			{Path: "generated.bin", Kind: types.VerificationWorktreeEffectUntrackedCreated, Ownership: "unproven_generated_artifact", Action: "retained_not_committed_not_auto_deleted"},
		},
	}
}

// Refused run: the verifier returns an error, verifyPostHook renders the
// failure surface — which must now carry the untracked lane and the
// disclosed lockfile row with its run-refused phrase, while the reason
// stays refused-rows-only.
func TestVerifyPostHook_RefusedRunRendersWorktreeAuditNote(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		t.Run(lang, func(t *testing.T) {
			plan, planPath := seedVerifyPlan(t, "plan-refused-"+lang)
			mu := types.NewMutableState("refused run")
			mu.SetChangePlan(plan)
			mu.SetChangeReport(&types.ChangeReport{
				PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: false,
				FailureKind: types.FailureKindVerificationSideEffect, FailureReasonCode: "verification_tracked_worktree_drift",
				FailureSummary: "verification command changed tracked worktree path(s): src/other.rs",
				TestResults:    []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "verification_worktree_integrity", Suite: "verification", Passed: false}},
				WorktreeAudit:  refusedRunAudit(),
			})
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, PlanPath: planPath, Language: lang}}
			if err := verifyPostHook(o, &agent.StageOutput{Error: "verify failed: verification command changed tracked worktree path(s): src/other.rs"}); err != nil {
				t.Fatalf("verifyPostHook: %v", err)
			}
			result := mu.Result()
			zh := lang == "zh"
			phrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenRunRefused, zh)
			for _, want := range []string{"`generated.bin`", "`Cargo.lock`", phrase} {
				if !strings.Contains(result, want) {
					t.Fatalf("lang=%s refused-run verify surface lost %q:\n%s", lang, want, result)
				}
			}
			if strings.Contains(result, "`src/other.rs`") {
				t.Fatalf("lang=%s refused rows belong to the failure reason, not the disclosed note:\n%s", lang, result)
			}
			if zh {
				if !strings.Contains(result, "测试未通过") || strings.Contains(result, "验证结论保持有效") {
					t.Fatalf("zh refused run must render the failure header and never say the verdict stands:\n%s", result)
				}
			} else if !strings.Contains(result, "Tests did not pass") || strings.Contains(result, "The verdict stands") {
				t.Fatalf("en refused run must render the failure header and never say the verdict stands:\n%s", result)
			}
			reloaded, err := types.LoadChangePlanFromFile(planPath)
			if err != nil || reloaded.Status != types.PlanStatusVerifyFailed {
				t.Fatalf("status = %v err=%v, want verify_failed", reloaded, err)
			}
		})
	}
}

// Unavailable verdict (changed-path coverage → verification_incomplete):
// verifyPostHook renders the unverified surface, which must carry the
// disclosed lockfile row and the untracked outputs too.
func TestVerifyPostHook_UnavailableVerdictRendersWorktreeAuditNote(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		t.Run(lang, func(t *testing.T) {
			plan, planPath := seedVerifyPlan(t, "plan-unavailable-"+lang)
			mu := types.NewMutableState("unavailable verdict")
			mu.SetChangePlan(plan)
			mu.SetChangeReport(&types.ChangeReport{
				PlanID: plan.ID, Channel: types.ChangeReportChannelPostApplyVerify, Passed: false,
				FailureKind: types.FailureKindVerificationIncomplete, FailureReasonCode: "changed_path_verification_uncovered",
				FailureSummary: "local verification did not cover changed source path(s): web/app.ts",
				TestResults:    []types.TestResult{{Kind: types.TestResultKindUnit, AssertionID: "project-tests", Suite: "project", Passed: true}},
				WorktreeAudit: &types.VerificationWorktreeAudit{
					Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, ReasonCode: types.VerificationTrackedSideEffectDisclosedReason,
					TrackedEffectCount: 1, UntrackedEffectCount: 1, DisclosedTrackedEffectCount: 1,
					Effects: []types.VerificationWorktreeEffect{
						{Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
							OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed, LockfileFixedPoint: types.VerificationLockfileFixedPointProven},
						{Path: "generated.bin", Kind: types.VerificationWorktreeEffectUntrackedCreated},
					},
				},
			})
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, PlanPath: planPath, Language: lang}}
			if err := verifyPostHook(o, &agent.StageOutput{}); err != nil {
				t.Fatalf("verifyPostHook: %v", err)
			}
			result := mu.Result()
			for _, want := range []string{"`generated.bin`", "`Cargo.lock`"} {
				if !strings.Contains(result, want) {
					t.Fatalf("lang=%s unverified surface lost %q:\n%s", lang, want, result)
				}
			}
			if lang == "zh" && !strings.Contains(result, "未完全验证") || lang == "en" && !strings.Contains(result, "Partially verified") {
				t.Fatalf("lang=%s must still render the unverified header:\n%s", lang, result)
			}
			if strings.Contains(result, "UNPROVEN") || strings.Contains(result, "未证明") {
				t.Fatalf("a proven fixed point must not be disclosed as unproven:\n%s", result)
			}
			reloaded, err := types.LoadChangePlanFromFile(planPath)
			if err != nil || reloaded.Status != types.PlanStatusUnverified {
				t.Fatalf("status = %v err=%v, want unverified", reloaded, err)
			}
		})
	}
}

// Note-level: a refused run's disclosed rows render with the refused wording
// (never "the verdict stands") in both languages, the run-refused fixed
// point is named, and refused rows stay out.
func TestRenderVerificationWorktreeAuditNoteRefusedRunNamesDisclosedRowsWithRefusedWording(t *testing.T) {
	audit := refusedRunAudit()
	for _, zh := range []bool{true, false} {
		note := renderVerificationWorktreeAuditNote(audit, zh)
		phrase := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenRunRefused, zh)
		for _, want := range []string{"`Cargo.lock`", phrase, "`generated.bin`"} {
			if !strings.Contains(note, want) {
				t.Fatalf("zh=%v note lost %q:\n%s", zh, want, note)
			}
		}
		if strings.Contains(note, "src/other.rs") {
			t.Fatalf("zh=%v refused rows belong to the failure surface:\n%s", zh, note)
		}
		if zh {
			if !strings.Contains(note, "已因其他路径的改动被拒绝") || strings.Contains(note, "保持有效") {
				t.Fatalf("zh refused wording expected:\n%s", note)
			}
		} else if !strings.Contains(note, "refused for other paths") || strings.Contains(note, "verdict stands") {
			t.Fatalf("en refused wording expected:\n%s", note)
		}
	}
	// All three renderers share the predicate: the same note text appears
	// in the success, failure and unverified surfaces for one report.
	report := &types.ChangeReport{PlanID: "plan-shared", Passed: true, WorktreeAudit: audit}
	note := renderVerificationWorktreeAuditNote(audit, false)
	for name, rendered := range map[string]string{
		"success":    renderVerifySuccess(report, "en"),
		"failure":    renderVerifyFailure(report, "verify failed: x", "en"),
		"unverified": renderVerifyUnverified(report, "en"),
	} {
		if !strings.Contains(rendered, note) {
			t.Fatalf("%s surface must render the shared audit note verbatim:\n%s", name, rendered)
		}
	}
}
