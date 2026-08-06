package writeflow

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestQualifyNoChangeReplanSentinelRequiresCurrentPlanAndFailureGeneration(t *testing.T) {
	handoffAt := time.Now()
	base := NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-current",
			FailureKind: types.FailureKindTestsFailed,
			GeneratedAt: handoffAt,
		},
		PriorPlan:          &types.ChangePlan{ID: "plan-current", AppliedCommitSHA: "abc123", TargetPaths: []string{"src/widget.py"}},
		RequireAppliedWork: true,
	}
	passingReport := func(planID string, generatedAt time.Time) *types.ChangeReport {
		return &types.ChangeReport{
			PlanID:             planID,
			Channel:            types.ChangeReportChannelPlannerProbe,
			VerificationStatus: types.VerificationStatusPassed,
			Passed:             true,
			GeneratedAt:        generatedAt,
			TestResults: []types.TestResult{{
				AssertionID: "probe",
				Kind:        types.TestResultKindUnit,
				Passed:      true,
			}},
			ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
				Path:       "src/widget.py",
				Status:     types.ChangedPathVerificationCovered,
				Capability: types.VerificationCapabilityTargetBehavior,
			}},
		}
	}
	base.PlannerProbeReports = []*types.ChangeReport{
		passingReport("plan-current", handoffAt.Add(-time.Millisecond)),
		passingReport("plan-older", handoffAt.Add(time.Millisecond)),
	}
	if q := QualifyNoChangeReplanSentinel(base); q.Allowed || q.ReasonCode != "planner_probe_missing" {
		t.Fatalf("stale/wrong-plan probes must not qualify: %+v", q)
	}
	currentAt := handoffAt.Add(2 * time.Millisecond)
	base.PlannerProbeReports = append(base.PlannerProbeReports, passingReport("plan-current", currentAt))
	q := QualifyNoChangeReplanSentinel(base)
	if !q.Allowed || q.ProbePlanID != "plan-current" || !q.ProbeGeneratedAt.Equal(currentAt) {
		t.Fatalf("current post-failure probe must qualify exactly: %+v", q)
	}
}

func TestQualifyNoChangeReplanSentinelAllowsPassedPlannerProbeForTestFailure(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-1",
			FailureKind: types.FailureKindTestsFailed,
		},
		PriorPlan: &types.ChangePlan{
			AppliedCommitSHA: "abc123",
			TargetPaths:      []string{"src/widget.py"},
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
			ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
				Path:       "src/widget.py",
				Status:     types.ChangedPathVerificationCovered,
				Capability: types.VerificationCapabilityTargetBehavior,
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
		PriorPlan: &types.ChangePlan{AppliedCommitSHA: "abc123", TargetPaths: []string{"src/widget.py"}},
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
			ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
				Path:       "src/widget.py",
				Status:     types.ChangedPathVerificationCovered,
				Capability: types.VerificationCapabilityTargetExecution,
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

func TestQualifyNoChangeReplanSentinelRejectsObservationOnlyProbe(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-1",
			FailureKind: types.FailureKindTestsFailed,
		},
		PriorPlan: &types.ChangePlan{ID: "plan-1", AppliedCommitSHA: "abc123"},
		PlannerProbeReports: []*types.ChangeReport{{
			PlanID:             "plan-1",
			Channel:            types.ChangeReportChannelPlannerProbe,
			VerificationStatus: types.VerificationStatusPassed,
			Passed:             true,
			TestResults: []types.TestResult{{
				AssertionID: "unbound-assert-true",
				Kind:        types.TestResultKindUnit,
				Passed:      true,
			}},
		}},
		RequireAppliedWork: true,
	})
	if q.Allowed || q.ReasonCode != "planner_probe_target_authority_missing" {
		t.Fatalf("an observation-only probe must not close a failed replan: %+v", q)
	}
}

func TestQualifyNoChangeReplanSentinelRejectsCoverageBorrowedFromCumulativeScope(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-rust",
			FailureKind: types.FailureKindTestsFailed,
		},
		PriorPlan: &types.ChangePlan{
			ID:               "plan-rust",
			AppliedCommitSHA: "abc123",
			TargetPaths:      []string{"src/lib.rs"},
			CumulativeVerificationScope: &types.CumulativeVerificationScope{
				TargetPaths: []string{"legacy/widget.py"},
			},
		},
		PlannerProbeReports: []*types.ChangeReport{{
			PlanID:             "plan-rust",
			Channel:            types.ChangeReportChannelPlannerProbe,
			VerificationStatus: types.VerificationStatusPassed,
			Passed:             true,
			TestResults:        []types.TestResult{{AssertionID: "legacy-python", Kind: types.TestResultKindUnit, Passed: true}},
			ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
				Path:       "legacy/widget.py",
				Status:     types.ChangedPathVerificationCovered,
				Capability: types.VerificationCapabilityTargetBehavior,
			}},
		}},
		RequireAppliedWork: true,
	})
	if q.Allowed || q.ReasonCode != "planner_probe_target_authority_missing" {
		t.Fatalf("coverage of a retained older path must not close the active plan failure: %+v", q)
	}
}

func TestQualifyNoChangeReplanSentinelAllowsUnavailableBuildRunnerAfterPassedProbe(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-1",
			FailureKind: types.FailureKindBuildFailure,
			Executed: []types.ExecutedCommand{{
				Runner:  "cmake",
				Outcome: "not_configured",
			}},
			Diagnostics: []types.VerificationDiagnostic{{
				Source:   "test_surface_default",
				Category: "environment",
				Severity: "warning",
				Runner:   "cmake",
				Outcome:  "not_configured",
			}},
		},
		PriorPlan: &types.ChangePlan{AppliedCommitSHA: "abc123", TargetPaths: []string{"src/widget.py"}},
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
			ChangedPathCoverage: []types.ChangedPathVerificationCoverage{{
				Path:       "src/widget.py",
				Status:     types.ChangedPathVerificationCovered,
				Capability: types.VerificationCapabilityTargetExecution,
			}},
		}},
		RequireAppliedWork: true,
	})
	if !q.Allowed {
		t.Fatalf("unavailable local build runner should be probe-resolvable, reason=%s detail=%s", q.ReasonCode, q.Detail)
	}
}

func TestQualifyNoChangeReplanSentinelRejectsRealBuildDiagnostic(t *testing.T) {
	q := QualifyNoChangeReplanSentinel(NoChangeReplanQualificationInput{
		VerifyFailureHandoff: &types.VerifyFailureHandoff{
			PlanID:      "plan-1",
			FailureKind: types.FailureKindBuildFailure,
			Diagnostics: []types.VerificationDiagnostic{{
				Source:     "compiler",
				Category:   "build",
				Severity:   "error",
				Outcome:    "failed",
				ReasonCode: "build_failed",
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
		t.Fatalf("real build diagnostic must require replacement replan: %+v", q)
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
