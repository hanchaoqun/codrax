package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestQualifyNoChangeReplanSentinelAllowsPassedPlannerProbeForTestFailure(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-1",
			FailureKind: types.FailureKindTestsFailed,
		},
		PriorPlan: &types.ChangePlan{
			AppliedCommitSHA: "abc123",
		},
		PlannerProbeReports: []*types.ChangeReport{{
			PlanID:             "plan-1",
			Channel:            types.ChangeReportChannelPlannerProbe,
			VerificationStatus: types.VerificationStatusPassed,
			Passed:             true,
			TestResults: []types.TestResult{{
				AssertionID: "probe",
				Kind:        types.TestResultKindUnit,
				Passed:      true,
			}},
		}},
		RequireAppliedWork: true,
	})
	if !q.Allowed {
		t.Fatalf("Allowed=false reason=%s detail=%s", q.ReasonCode, q.Detail)
	}
}

func TestQualifyNoChangeReplanSentinelRejectsBuildFailure(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-1",
			FailureKind: types.FailureKindBuildFailure,
		},
		PriorPlan: &types.ChangePlan{AppliedCommitSHA: "abc123"},
		PlannerProbeReports: []*types.ChangeReport{{
			PlanID:             "plan-1",
			Channel:            types.ChangeReportChannelPlannerProbe,
			VerificationStatus: types.VerificationStatusPassed,
			Passed:             true,
			TestResults: []types.TestResult{{
				AssertionID: "probe",
				Kind:        types.TestResultKindUnit,
				Passed:      true,
			}},
		}},
		RequireAppliedWork: true,
	})
	if q.Allowed {
		t.Fatalf("build failure must not be probe-resolved: %+v", q)
	}
	if q.ReasonCode != "verify_failure_kind_not_probe_resolvable" {
		t.Fatalf("ReasonCode=%q", q.ReasonCode)
	}
}

func TestQualifyNoChangeReplanSentinelRejectsPatchReviewHardFailure(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:            "plan-1",
			FailureKind:       types.FailureKindTestsFailed,
			FailureReasonCode: "python_unreachable_body_after_added_return",
			Diagnostics: []types.VerificationDiagnostic{{
				Source:     "patch_review",
				Category:   "structural",
				Severity:   "error",
				ReasonCode: "python_unreachable_body_after_added_return",
				Outcome:    "failed",
			}},
		},
		PriorPlan: &types.ChangePlan{AppliedCommitSHA: "abc123"},
		PlannerProbeReports: []*types.ChangeReport{{
			PlanID:             "plan-1",
			Channel:            types.ChangeReportChannelPlannerProbe,
			VerificationStatus: types.VerificationStatusPassed,
			Passed:             true,
			TestResults: []types.TestResult{{
				AssertionID: "probe",
				Kind:        types.TestResultKindUnit,
				Passed:      true,
			}},
		}},
		RequireAppliedWork: true,
	})
	if q.Allowed {
		t.Fatalf("patch-review hard failure must not be probe-resolved: %+v", q)
	}
	if q.ReasonCode != "patch_review_hard_failure_requires_replacement_patch" {
		t.Fatalf("ReasonCode=%q", q.ReasonCode)
	}
}

func TestQualifyNoChangeReplanSentinelRejectsWeakPlannerProbeConfidence(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-1",
			FailureKind: types.FailureKindTestsFailed,
		},
		PriorPlan: &types.ChangePlan{AppliedCommitSHA: "abc123"},
		PlannerProbeReports: []*types.ChangeReport{{
			PlanID:             "plan-1",
			Channel:            types.ChangeReportChannelPlannerProbe,
			VerificationStatus: types.VerificationStatusPassed,
			Passed:             true,
			TestResults: []types.TestResult{{
				AssertionID: "probe",
				Kind:        types.TestResultKindUnit,
				Passed:      true,
			}},
			VerificationConfidence: []types.VerificationConfidenceRecord{{
				Severity:   "warning",
				Status:     "missing",
				ReasonCode: "verification_probe_missing_changed_symbol_ref",
			}},
		}},
		RequireAppliedWork: true,
	})
	if q.Allowed {
		t.Fatalf("weak confidence probe must not authorize no-change: %+v", q)
	}
	if q.ReasonCode != "verification_probe_missing_changed_symbol_ref" {
		t.Fatalf("ReasonCode=%q", q.ReasonCode)
	}
}
