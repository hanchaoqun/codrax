package types

import "testing"

func TestBuildVerificationProofProfileStrongProjectRunner(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-strong",
		PatchReview: &PatchReviewRecord{Findings: []PatchReviewFinding{{
			Code:           "changed_symbol_without_probe_coverage",
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageVerified,
		}}},
		ImpactAnalysis: &ImpactAnalysisResult{VerificationTargets: []ImpactVerificationTarget{{
			Kind:           "changed_symbol",
			Path:           "src/app.go",
			CoverageStatus: "verified",
		}}},
		LocalizationReview: &SourceLocalizationReview{
			Status:         SourceLocalizationSupported,
			SourcePaths:    []string{"src/app.go"},
			SupportedPaths: []string{"src/app.go"},
		},
	}
	report := &ChangeReport{
		PlanID:             "plan-strong",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			AssertionID: "TestApp",
			Suite:       "pkg/app",
			Passed:      true,
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "go",
			Suite:   "./...",
			Command: "go test ./...",
			Outcome: "executed",
			Source:  "auto_detect",
		}},
	}

	got := BuildVerificationProofProfile(plan, report)
	if got.Status != VerificationProofStrong || got.RunnerEvidence != VerificationProofRunnerProject {
		t.Fatalf("profile=%+v, want strong project runner", got)
	}
	if got.TestCount != 1 || got.ProjectRunnerCommands != 1 || got.ImpactVerifiedCount != 1 {
		t.Fatalf("profile counts not projected: %+v", got)
	}
	if len(got.ReasonCodes) != 0 {
		t.Fatalf("strong proof should not carry risk reasons: %+v", got.ReasonCodes)
	}
}

func TestBuildVerificationProofProfileWeakProbeOnly(t *testing.T) {
	plan := &ChangePlan{
		ID: "plan-weak",
		PatchReview: &PatchReviewRecord{Findings: []PatchReviewFinding{{
			Code:           "behavior_contract_without_verify_coverage",
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageUnverified,
			ImpactKind:     PatchReviewImpactKindBehaviorContract,
		}}},
		ImpactAnalysis: &ImpactAnalysisResult{VerificationTargets: []ImpactVerificationTarget{{
			Kind:           "behavior_contract",
			ContractRef:    "contract-1",
			CoverageStatus: "unverified",
		}}},
		LocalizationReview: &SourceLocalizationReview{
			Status:       SourceLocalizationWeak,
			SourcePaths:  []string{"src/app.py"},
			MissingPaths: []string{"src/app.py"},
			ReasonCodes:  []string{"plan_source_paths_without_prior_context"},
		},
	}
	report := &ChangeReport{
		PlanID:             "plan-weak",
		Passed:             true,
		VerificationStatus: VerificationStatusPassed,
		TestResults: []TestResult{{
			AssertionID: "probe/python",
			Suite:       "verification_probe/python",
			Passed:      true,
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:  "verification_probe",
			Suite:   "verification_probe/python",
			Outcome: "executed",
			Source:  "python_verification_probe",
		}},
		VerificationConfidence: []VerificationConfidenceRecord{{
			Source:     "verification_probe",
			Category:   "probe_contract_refs",
			Status:     "missing",
			ReasonCode: "verification_probe_missing_required_contract_ref",
		}},
	}

	got := BuildVerificationProofProfile(plan, report)
	if got.Status != VerificationProofWeak || got.RunnerEvidence != VerificationProofRunnerVerificationProbe {
		t.Fatalf("profile=%+v, want weak verification probe", got)
	}
	for _, want := range []string{
		"verification_probe_missing_required_contract_ref",
		"patch_review_semantic_unverified",
		"impact_targets_unverified",
		"source_localization_weak",
	} {
		if !verificationProofHasReason(got, want) {
			t.Fatalf("profile reasons %+v missing %q", got.ReasonCodes, want)
		}
	}
}

func TestBuildVerificationProofProfileUnavailable(t *testing.T) {
	report := &ChangeReport{
		PlanID:             "plan-unavailable",
		Passed:             true,
		VerificationStatus: VerificationStatusUnavailable,
		FailureKind:        FailureKindRunnerMissing,
		FailureReasonCode:  "pytest_missing",
	}

	got := BuildVerificationProofProfile(nil, report)
	if got.Status != VerificationProofUnavailable ||
		got.RunnerEvidence != VerificationProofRunnerUnavailable ||
		!verificationProofHasReason(got, "pytest_missing") {
		t.Fatalf("profile=%+v, want unavailable pytest_missing", got)
	}
}

func verificationProofHasReason(profile VerificationProofProfile, code string) bool {
	for _, reason := range profile.ReasonCodes {
		if reason == code {
			return true
		}
	}
	return false
}
