package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// write_controller_patch_review_outcome_fold_in5_test.go — fold-in round
// five (colleague_merge_audit §40.36 五轮收编, finding CC): the post-apply
// patch review publishes an ExecutedCommand row and a VerificationDiagnostic
// with Outcome "failed". The label predates the closed ExecutedCommandOutcome
// set and used to route through every consumer's unknown-label default arm;
// it is now the member types.ExecutedCommandOutcomeFailed (the patch-review
// lane), decided explicitly by every consumer. This pin drives the REAL
// producer (patchReviewFailureReport) and asserts the decisions:
//   - failed command: true (ExecutedCommandFailed);
//   - unavailable reason: none (ExecutedCommandUnavailableReasonCode "");
//   - FailureKind: the report's own (tests_failed is retained; the row names
//     no kind of its own);
//   - proof ledger: a failed executed_command capability (never unavailable);
//   - the diagnostic keeps the patch-review category and is a failed
//     diagnostic for the replan gate.
func TestPatchReviewFailureReportRowIsTheFailedMemberDecidedByEveryConsumer(t *testing.T) {
	plan := &types.ChangePlan{ID: "plan-cc", Status: types.PlanStatusApplied, TargetPaths: []string{"pkg/a.go"}}
	review := types.PatchReviewRecord{
		PlanID:    "plan-cc",
		Status:    "failed",
		HardBlock: true,
		Findings: []types.PatchReviewFinding{{
			Code:     "patch_review_symbol_removed",
			Severity: types.PatchReviewSeverityError,
			Category: types.PatchReviewCategoryStructural,
			Message:  "exported symbol removed without replacement",
			Path:     "pkg/a.go",
		}},
	}
	report := patchReviewFailureReport(plan, review, "")
	if report == nil || len(report.ExecutedCommands) != 1 || len(report.VerificationDiagnostics) != 1 {
		t.Fatalf("patch review report shape: %+v", report)
	}
	row := report.ExecutedCommands[0]
	diag := report.VerificationDiagnostics[0]
	if row.Outcome != types.ExecutedCommandOutcomeFailed || diag.Outcome != types.ExecutedCommandOutcomeFailed {
		t.Fatalf("producer must write the failed member on both the row and the diagnostic: row=%q diag=%q", row.Outcome, diag.Outcome)
	}
	if !types.ExecutedCommandFailed(row) {
		t.Fatalf("the patch-review row is a failed command: %+v", row)
	}
	if code := types.ExecutedCommandUnavailableReasonCode(row); code != "" {
		t.Fatalf("the patch-review row is never an unavailable reason, got %q", code)
	}
	if report.FailureKind != types.FailureKindTestsFailed || report.NormalizeVerificationStatus() != types.VerificationStatusFailed {
		t.Fatalf("the report keeps its own failure kind: kind=%s status=%s", report.FailureKind, report.NormalizeVerificationStatus())
	}
	if code := report.VerificationUnavailableReasonCode(); code != "" {
		t.Fatalf("a patch-review failure is not verification-unavailable, got %q", code)
	}
	ledger := types.BuildVerificationProofLedger(plan, report, nil)
	rowFailed := false
	for _, item := range ledger.Capabilities {
		if item.Kind != "executed_command" {
			continue
		}
		if item.Status != types.VerificationProofLedgerItemFailed {
			t.Fatalf("patch-review capability status = %s, want failed: %+v", item.Status, item)
		}
		rowFailed = true
	}
	if !rowFailed || ledger.CapabilityFailedCount == 0 || ledger.CapabilityUnavailableCount != 0 {
		t.Fatalf("ledger must record the patch-review row as a failed capability: failed=%d unavailable=%d items=%+v",
			ledger.CapabilityFailedCount, ledger.CapabilityUnavailableCount, ledger.Capabilities)
	}
	if diag.Category != string(types.PatchReviewCategoryStructural) || diag.Severity != string(types.PatchReviewSeverityError) {
		t.Fatalf("diagnostic class must be the patch-review category at error severity: %+v", diag)
	}
	handoff := &types.VerifyFailureHandoff{PlanID: "plan-cc", Diagnostics: report.VerificationDiagnostics}
	if got := writeflow.VerifyFailureRequiresReplacementPatch(handoff); got != "patch_review_hard_failure_requires_replacement_patch" {
		t.Fatalf("the failed patch-review diagnostic must require a replacement patch, got %q", got)
	}
	// The row is not a launched process for the coverage / untried-candidate
	// consumers.
	if verifyCoverageCommandCoversPath(row) {
		t.Fatalf("a patch-review row covers no changed path")
	}
	report.TestSurface = &types.TestSurface{Candidates: []types.TestSurfaceCandidate{{Runner: "patch_review", WorkingDir: ".", HasTestSignal: true}}}
	if cand := reportUntriedRunnableCandidate(report); cand != nil {
		t.Fatalf("a patch-review row must not license an 'untried runnable candidate' wording: %+v", cand)
	}
	if !strings.Contains(report.FailureSummary, "patch review structural finding requires replan") {
		t.Fatalf("summary: %q", report.FailureSummary)
	}
}
