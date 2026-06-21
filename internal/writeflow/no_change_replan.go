package writeflow

import (
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// NoChangeReplanQualification is the deterministic verdict for accepting a
// no_change_required sentinel after a failed post-apply verify. Tool and
// scheduler callers share this helper so the model cannot get a tool-level
// success that the controller later rejects under a different policy.
type NoChangeReplanQualification struct {
	Allowed     bool
	ReasonCode  string
	Detail      string
	ProbePlanID string
}

type NoChangeReplanQualificationInput struct {
	VerifyFailureHandoff *types.VerifyFailureHandoff
	PriorPlan            *types.ChangePlan
	PlannerProbeReports  []*types.ChangeReport
	RequireAppliedWork   bool
}

func QualifyNoChangeReplanSentinel(in NoChangeReplanQualificationInput) NoChangeReplanQualification {
	handoff := in.VerifyFailureHandoff
	if handoff == nil || strings.TrimSpace(handoff.PlanID) == "" {
		return noChangeReplanDenied("verify_failure_handoff_missing", "no failed post-apply verify handoff is available")
	}
	if in.RequireAppliedWork && !changePlanHasAppliedWork(in.PriorPlan) {
		return noChangeReplanDenied("prior_plan_not_applied", "no applied prior plan is available for the no-change sentinel")
	}
	if reason := verifyFailureHandoffProbeResolvableReason(handoff); reason != "" {
		return noChangeReplanDenied(reason, "the previous verify failure requires a real replan or environment resolution")
	}
	if reason := VerifyFailureRequiresReplacementPatch(handoff); reason != "" {
		return noChangeReplanDenied(reason, "the previous verify failure is a typed patch-review hard failure and requires a replacement patch, not a no-change probe sentinel")
	}
	report := latestPlannerProbeReport(in.PlannerProbeReports)
	if report == nil {
		return noChangeReplanDenied("planner_probe_missing", "no typed planner probe report is available")
	}
	if report.NormalizeVerificationStatus() != types.VerificationStatusPassed {
		return noChangeReplanDenied("planner_probe_not_passed", "the latest typed planner probe did not pass")
	}
	passed, total := report.Score()
	if total <= 0 || passed != total {
		return noChangeReplanDenied("planner_probe_no_passing_assertions", "the latest typed planner probe has no complete passing assertion set")
	}
	if weak := weakPlannerProbeConfidenceReason(report.VerificationConfidence); weak != "" {
		return noChangeReplanDenied(weak, "the latest typed planner probe passed but carries warning-level coverage or confidence gaps")
	}
	return NoChangeReplanQualification{
		Allowed:     true,
		ReasonCode:  "planner_probe_passed_existing_worktree",
		Detail:      "latest typed planner probe passed against an already-applied worktree",
		ProbePlanID: strings.TrimSpace(report.PlanID),
	}
}

func verifyFailureHandoffProbeResolvableReason(handoff *types.VerifyFailureHandoff) string {
	if handoff == nil {
		return "verify_failure_handoff_missing"
	}
	switch handoff.FailureKind {
	case "", types.FailureKindTestsFailed,
		types.FailureKindRunnerMissing,
		types.FailureKindParserError,
		types.FailureKindNoTests,
		types.FailureKindPreexistingBuildFailure:
		return ""
	case types.FailureKindBuildFailure:
		if types.FailureReasonCodeIndicatesVerificationUnavailable(handoff.FailureReasonCode) ||
			verifyFailureHandoffHasOnlyUnavailableVerifierEvidence(handoff) {
			return ""
		}
		return "verify_failure_kind_not_probe_resolvable"
	default:
		return "verify_failure_kind_not_probe_resolvable"
	}
}

func verifyFailureHandoffHasOnlyUnavailableVerifierEvidence(handoff *types.VerifyFailureHandoff) bool {
	if handoff == nil {
		return false
	}
	seenUnavailable := false
	for _, cmd := range handoff.Executed {
		if code := types.ExecutedCommandUnavailableReasonCode(cmd); code != "" {
			seenUnavailable = true
			continue
		}
		if verifyFailureCommandLooksFailed(cmd) {
			return false
		}
	}
	for _, diag := range handoff.Diagnostics {
		if code := verifyFailureDiagnosticUnavailableReasonCode(diag); code != "" {
			seenUnavailable = true
			continue
		}
		if verifyFailureDiagnosticLooksFailed(diag) {
			return false
		}
	}
	for _, rec := range handoff.Confidence {
		if strings.TrimSpace(rec.Status) != "unavailable" {
			continue
		}
		if types.FailureReasonCodeIndicatesVerificationUnavailable(rec.ReasonCode) {
			seenUnavailable = true
			continue
		}
	}
	if len(handoff.FailingTests) > 0 || len(handoff.BuildErrors) > 0 {
		return false
	}
	return seenUnavailable
}

func verifyFailureCommandLooksFailed(cmd types.ExecutedCommand) bool {
	switch strings.TrimSpace(cmd.Outcome) {
	case "", "executed", "syntax_preflight", "syntax_check_fallback", "suite_continued", "suite_skipped":
		return cmd.ExitCode != 0
	case "synthetic_no_tests", "zero_tests":
		return false
	default:
		return true
	}
}

func verifyFailureDiagnosticUnavailableReasonCode(diag types.VerificationDiagnostic) string {
	if types.FailureReasonCodeIndicatesVerificationUnavailable(diag.ReasonCode) {
		return strings.TrimSpace(diag.ReasonCode)
	}
	switch strings.TrimSpace(diag.Outcome) {
	case "not_configured", "runner_missing", "parser_error", "suite_skipped":
		return strings.TrimSpace(diag.Outcome)
	}
	return ""
}

func verifyFailureDiagnosticLooksFailed(diag types.VerificationDiagnostic) bool {
	switch strings.TrimSpace(diag.Outcome) {
	case "", "passed", "available", "suite_skipped":
		return false
	case "not_configured", "runner_missing", "parser_error":
		return false
	}
	return strings.TrimSpace(diag.Severity) == "error" ||
		strings.TrimSpace(diag.Outcome) == "failed"
}

func VerifyFailureRequiresReplacementPatch(handoff *types.VerifyFailureHandoff) string {
	if handoff == nil {
		return ""
	}
	for _, diag := range handoff.Diagnostics {
		if strings.TrimSpace(diag.Source) != "patch_review" {
			continue
		}
		if strings.TrimSpace(diag.Severity) != "error" {
			continue
		}
		if outcome := strings.TrimSpace(diag.Outcome); outcome != "" && outcome != "failed" {
			continue
		}
		return "patch_review_hard_failure_requires_replacement_patch"
	}
	return ""
}

func noChangeReplanDenied(code, detail string) NoChangeReplanQualification {
	return NoChangeReplanQualification{
		Allowed:    false,
		ReasonCode: strings.TrimSpace(code),
		Detail:     strings.TrimSpace(detail),
	}
}

func latestPlannerProbeReport(reports []*types.ChangeReport) *types.ChangeReport {
	for i := len(reports) - 1; i >= 0; i-- {
		report := reports[i]
		if report == nil || report.Channel != types.ChangeReportChannelPlannerProbe {
			continue
		}
		return report
	}
	return nil
}

func weakPlannerProbeConfidenceReason(records []types.VerificationConfidenceRecord) string {
	for _, rec := range records {
		if strings.TrimSpace(rec.Severity) != "warning" {
			continue
		}
		switch strings.TrimSpace(rec.Status) {
		case "missing", "unavailable":
			if code := strings.TrimSpace(rec.ReasonCode); code != "" {
				return code
			}
			return "planner_probe_confidence_warning"
		}
	}
	return ""
}

func changePlanHasAppliedWork(plan *types.ChangePlan) bool {
	if plan == nil {
		return false
	}
	if strings.TrimSpace(plan.AppliedCommitSHA) != "" || strings.TrimSpace(plan.WorktreePath) != "" {
		return true
	}
	for _, change := range plan.Changes {
		if change.Apply != nil && strings.TrimSpace(change.Apply.Status) == "applied" {
			return true
		}
	}
	return false
}
