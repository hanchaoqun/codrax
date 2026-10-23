package orchestrator

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// Complete only the independent apply coordinates using the actual public
// tool/Git delivery. The old native test remains historical; no source
// controller verify attempt or completion is invented or removed.
func completeControllerRegistrationApplyReceipt(t *testing.T, f controllerRegistrationFixture) {
	t.Helper()
	delivery, ok := types.VerificationDeliverySnapshotFromAppliedPlan(f.source)
	if !ok || !delivery.Valid() || delivery.AppliedCommitSHA != f.head ||
		runGitForWorkflowRestoreTest(t, f.o.busCtx.RepoRoot, "rev-parse", "HEAD") != f.head+"\n" {
		t.Fatal("apply receipt requires the current actual source delivery")
	}
	run := f.o.busCtx.Mutable.WriteWorkflowRun()
	identity := f.o.currentWriteWorkflowRepoIdentity(f.o.currentWriteWorkflowGoalHashSource())
	run.Identity = &identity
	count := 0
	for i := range run.Batches {
		source := &run.Batches[i]
		if source.PlanID != f.source.ID {
			continue
		}
		count++
		if source.Completion != nil || source.VerifyRef != "" || len(source.Attempts) != 1 ||
			source.Attempts[0].Kind != "apply" || source.Attempts[0].Status != "applied" || source.Attempts[0].PlanID != f.source.ID {
			t.Fatal("unverified source fixture already contains another attempt or terminal")
		}
		source.ApplyRef = delivery.AppliedCommitSHA
		source.Attempts[0].ArtifactRef = delivery.AppliedCommitSHA
	}
	if count != 1 {
		t.Fatal("source apply must be unique")
	}
	f.o.busCtx.Mutable.SetWriteWorkflowRun(run)
}

// Real source reads do not create a new edit obligation in a strictly
// read-only registration. The source's absent verification remains absent.
func TestHMC287NativeRegistrationMissingSourceTerminalScopePublic(t *testing.T) {
	f := newControllerRegistrationFixture(t)
	completeControllerRegistrationApplyReceipt(t, f)
	f.register(t, "emit_change_plan", true)
	mu := f.o.busCtx.Mutable
	run, plan := mu.WriteWorkflowRun(), mu.ChangePlan()
	if loopkernel.WorkflowRegistrationReusesVerifiedSource(*run, plan) {
		t.Fatal("unverified source was granted strict verified-source authority")
	}
	var source *types.WriteWorkflowBatch
	for i := range run.Batches {
		if run.Batches[i].PlanID == f.source.ID {
			if source != nil {
				t.Fatal("source batch must be unique")
			}
			source = &run.Batches[i]
		}
	}
	if source == nil || source.Completion != nil || source.ApplyRef != f.head || source.VerifyRef != "" {
		t.Fatalf("source is not exactly applied but unverified: %+v", source)
	}
	applied := false
	for _, attempt := range source.Attempts {
		if attempt.Kind == "verify" {
			t.Fatal("source verification was unexpectedly introduced")
		}
		if attempt.Kind == "apply" && attempt.Status == "applied" && attempt.PlanID == f.source.ID && attempt.ArtifactRef == f.head {
			applied = true
		}
	}
	if !applied {
		t.Fatal("source lacks its exact independent apply attempt")
	}
	view := writeflow.DeriveWorkflowExecutionView(types.ModeApply, *run, plan)
	t.Logf("isolated absent source verification: apply_ref=%s verify_ref=%s localization=%+v", source.ApplyRef, source.VerifyRef, view.Localization)
	// This helper checks a fresh invocation, unchanged source/tests, verified
	// current proof and unverified overall/missing_terminal_verify_verdict.
	f.restart(t)
}

func TestHMC287NativeRegistrationSourceScopeBoundariesPublic(t *testing.T) {
	f := registrationSourceReadFixture(t, "emit_change_plan")
	mu := f.o.busCtx.Mutable
	original, err := json.Marshal(struct {
		Run  *types.WriteWorkflowRun
		Plan *types.ChangePlan
	}{mu.WriteWorkflowRun(), mu.ChangePlan()})
	if err != nil {
		t.Fatal(err)
	}
	for _, condition := range []string{
		"verified", "backfilled_terminal", "source_verification_absent", "earlier_failed_then_current_pass", "empty_terminal", "unverified_owner", "accepted_failure",
		"missing_apply_ref", "missing_apply_artifact", "dangling_verify_ref", "latest_failed_with_old_pass_ref", "verify_not_executed",
		"unverified_attempt_without_terminal", "dangling_ref_without_attempt", "extra_current_owner", "new_source_edit", "missing_registration_receipt",
	} {
		t.Run(condition, func(t *testing.T) {
			var input struct {
				Run  *types.WriteWorkflowRun
				Plan *types.ChangePlan
			}
			if err := json.Unmarshal(original, &input); err != nil {
				t.Fatal(err)
			}
			run, plan := input.Run, input.Plan
			var source *types.WriteWorkflowBatch
			for i := range run.Batches {
				if run.Batches[i].PlanID == f.source.ID {
					source = &run.Batches[i]
				}
			}
			if source == nil {
				t.Fatal("source is absent")
			}
			if condition != "verified" {
				source.Completion = nil
			}
			switch condition {
			case "source_verification_absent", "dangling_ref_without_attempt":
				var attempts []types.WriteWorkflowAttempt
				for _, attempt := range source.Attempts {
					if attempt.Kind != "verify" {
						attempts = append(attempts, attempt)
					}
				}
				source.Attempts = attempts
				if condition == "source_verification_absent" {
					source.VerifyRef = ""
				}
			case "earlier_failed_then_current_pass":
				previous := types.WriteWorkflowAttempt{Kind: "verify", Status: "failed", PlanID: source.PlanID, ReportID: "earlier-failed-report"}
				source.Attempts = append([]types.WriteWorkflowAttempt{previous}, source.Attempts...)
			case "empty_terminal":
				source.Completion = &types.WriteWorkflowCompletion{}
			case "unverified_owner":
				source.Completion = &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionUnverified, Source: "verify_attempt", ReasonCode: "localization_observed_without_owner"}
			case "accepted_failure":
				source.Completion = &types.WriteWorkflowCompletion{Verdict: types.WriteWorkflowCompletionAcceptedFailed, Source: "verify_attempt"}
			case "missing_apply_ref":
				source.ApplyRef = ""
			case "missing_apply_artifact":
				for i := range source.Attempts {
					if source.Attempts[i].Kind == "apply" {
						source.Attempts[i].ArtifactRef = ""
					}
				}
			case "dangling_verify_ref":
				source.VerifyRef = "not-the-current-report"
			case "latest_failed_with_old_pass_ref", "verify_not_executed", "unverified_attempt_without_terminal":
				status := "failed"
				if condition == "verify_not_executed" {
					status = "started"
				} else if condition == "unverified_attempt_without_terminal" {
					status = "unverified"
				}
				source.Attempts = append(source.Attempts, types.WriteWorkflowAttempt{Kind: "verify", Status: status, PlanID: source.PlanID, ReportID: source.VerifyRef})
			case "extra_current_owner":
				plan.LocalizationReview = &types.SourceLocalizationReview{PlanID: plan.ID, BatchID: run.ActiveBatchID, SourcePaths: []string{"value.py"}, OwnerMissingPaths: []string{"value.py"}}
			case "new_source_edit":
				plan.Changes = []types.FileChange{{Path: "value.py", Kind: "modify", NewContent: "changed"}}
			case "missing_registration_receipt":
				run.NativeTestRegistrations = nil
			}
			before, _ := json.Marshal(input)
			wantNoNewOwner := condition == "verified" || condition == "backfilled_terminal" || condition == "source_verification_absent" || condition == "earlier_failed_then_current_pass"
			if got := loopkernel.WorkflowRegistrationHasNoNewSourceOwnerObligation(*run, plan); got != wantNoNewOwner {
				t.Fatalf("no-new-owner scope=%t, want %t", got, wantNoNewOwner)
			}
			if got := loopkernel.WorkflowRegistrationReusesVerifiedSource(*run, plan); got != (condition == "verified") {
				t.Fatalf("strict verified-source authority changed: %t", got)
			}
			view := writeflow.DeriveWorkflowExecutionView(types.ModeApply, *run, plan)
			if wantNoNewOwner {
				if view.Localization.State != "" || view.LocalizationGateEligible || view.Localization.RequiresMoreContext {
					t.Fatalf("registration acquired owner authority or new obligation: %+v", view.Localization)
				}
			} else if !view.Localization.RequiresMoreContext {
				t.Fatalf("invalid history or current owner obligation was dropped: %+v", view.Localization)
			}
			if condition == "backfilled_terminal" || condition == "source_verification_absent" || condition == "unverified_attempt_without_terminal" || condition == "latest_failed_with_old_pass_ref" || condition == "verify_not_executed" || condition == "dangling_ref_without_attempt" {
				normalized := types.NormalizeWriteWorkflowRun(*run)
				if got := loopkernel.WorkflowRegistrationHasNoNewSourceOwnerObligation(normalized, plan); got != wantNoNewOwner {
					t.Fatalf("normalized source scope=%t, want %t", got, wantNoNewOwner)
				}
				for _, batch := range normalized.Batches {
					if batch.PlanID == f.source.ID && condition == "backfilled_terminal" && (batch.Completion == nil || batch.Completion.Verdict != types.WriteWorkflowCompletionVerified || batch.Completion.Source != "attempts_backfill") {
						t.Fatal("legacy successful verification backfill was not preserved")
					}
					if batch.PlanID == f.source.ID && condition == "source_verification_absent" && batch.Completion != nil {
						t.Fatal("absent source verification was manufactured")
					}
				}
			}
			after, _ := json.Marshal(input)
			if string(before) != string(after) {
				t.Fatal("scope projection rewrote the source terminal, receipts, or read anchors")
			}
		})
	}
}

// A distinct operator opt-in keeps the original incomplete-source fixture and
// its historical failure reproducible. Exact source apply coordinates are
// retained, while source controller verification is genuinely absent.
func TestNativeRegistrationLiveRestoredMissingTerminalReceipt(t *testing.T) {
	runNativeRegistrationLiveEvaluationWithScope(t, os.Getenv("CODRAX_LIVE_REGISTRATION_MISSING_TERMINAL_EVAL_DIR"), false, false, true)
}
