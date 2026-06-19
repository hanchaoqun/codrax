package worker

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/loopkernel"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunImpactAnalyzerWrapsImpactArtifact(t *testing.T) {
	out := RunImpactAnalyzer(ImpactAnalyzerInput{
		ID: "impact-1",
		Analysis: &types.ImpactAnalysisResult{
			PlanID: "plan-1",
			ChangedSurfaces: []types.ImpactChangedSurface{{
				Kind:        "symbol",
				Path:        "pkg/a.py",
				Symbol:      "A",
				EvidenceRef: "impact-ev-1",
			}},
			VerificationTargets: []types.ImpactVerificationTarget{{
				Kind:        "dependent",
				Path:        "pkg/a.py",
				RelatedPath: "pkg/b.py",
				EvidenceRef: "impact-ev-2",
			}},
		},
	})
	if out.Result.ReasonCode != "impact_analysis_targets_projected" {
		t.Fatalf("reason = %q", out.Result.ReasonCode)
	}
	if !stringSliceContains(out.Result.EvidenceRefs, "impact-ev-1") ||
		!stringSliceContains(out.Result.EvidenceRefs, "impact-ev-2") {
		t.Fatalf("impact evidence refs not carried: %+v", out.Result.EvidenceRefs)
	}
	if !artifactKindsContain(out.Result.OutputRefs, "impact_analysis_result") {
		t.Fatalf("impact output ref missing: %+v", out.Result.OutputRefs)
	}
	assertWorkerResultValid(t, out.Request, out.Result)
}

func TestRunPatchCriticWrapsActualReviewArtifact(t *testing.T) {
	out := RunPatchCritic(PatchCriticInput{
		ID: "critic-1",
		Review: &types.PatchReviewRecord{
			ReviewID:     "review-1",
			TargetPaths:  []string{"pkg/a.py"},
			AppliedPaths: []string{"pkg/a.py"},
			Findings: []types.PatchReviewFinding{{
				Code:           "changed_symbol_unverified",
				Severity:       types.PatchReviewSeverityWarning,
				Category:       types.PatchReviewCategorySemanticCoverage,
				ImpactKind:     types.PatchReviewImpactKindChangedSymbol,
				CoverageStatus: types.PatchReviewCoverageUnverified,
				Path:           "pkg/a.py",
				EvidenceRef:    "review-ev-1",
			}},
		},
	})
	if out.Summary.Verdict != types.PatchReviewCoverageVerdictUnverified {
		t.Fatalf("summary = %+v, want unverified", out.Summary)
	}
	if out.Result.ReasonCode != "patch_review_unverified" {
		t.Fatalf("reason = %q", out.Result.ReasonCode)
	}
	if !stringSliceContains(out.Result.EvidenceRefs, "review-ev-1") {
		t.Fatalf("review evidence ref not carried: %+v", out.Result.EvidenceRefs)
	}
	assertWorkerResultValid(t, out.Request, out.Result)
}

func TestRunProofAuditorBuildsLedgerAndAuthority(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Changes:     []types.FileChange{{Path: "pkg/a.py", Kind: "modify", Rationale: "repair"}},
		TargetPaths: []string{"pkg/a.py"},
	}
	report := &types.ChangeReport{
		PlanID:             "plan-1",
		Passed:             true,
		VerificationStatus: types.VerificationStatusPassed,
		TestResults: []types.TestResult{{
			AssertionID: "test_a",
			Suite:       "pkg",
			Passed:      true,
		}},
	}
	out := RunProofAuditor(ProofAuditorInput{ID: "proof-1", Plan: plan, Report: report})
	if out.Authority.State != loopkernel.ProofCoverageCovered {
		t.Fatalf("proof authority = %+v, want covered", out.Authority)
	}
	if !artifactKindsContain(out.Result.OutputRefs, "verification_proof_ledger") ||
		!artifactKindsContain(out.Result.OutputRefs, "proof_coverage_authority") {
		t.Fatalf("proof output refs missing: %+v", out.Result.OutputRefs)
	}
	assertWorkerResultValid(t, out.Request, out.Result)
}

func TestRunFailureAnalyzerBuildsTypedHandoff(t *testing.T) {
	report := &types.ChangeReport{
		PlanID:            "plan-1",
		Passed:            false,
		FailureKind:       types.FailureKindBuildFailure,
		FailureReasonCode: "python_syntax_error",
		TestResults: []types.TestResult{{
			Kind:        types.TestResultKindBuildError,
			AssertionID: "build",
			Suite:       "build",
			Passed:      false,
			BuildErrors: []types.BuildError{{
				File:    "pkg/a.py",
				Line:    10,
				Message: "invalid syntax",
			}},
		}},
	}
	out := RunFailureAnalyzer(FailureAnalyzerInput{
		ID:                 "failure-1",
		BatchID:            "batch-1",
		Attempt:            2,
		Report:             report,
		DiffArtifactRef:    "plan-1.attempt-2.diff",
		SurfaceArtifactRef: "plan-1.attempt-2.surface.json",
	})
	if out.Handoff == nil || out.Handoff.FailureReasonCode != "python_syntax_error" {
		t.Fatalf("handoff = %+v", out.Handoff)
	}
	if out.Result.ReasonCode != "python_syntax_error" {
		t.Fatalf("reason = %q", out.Result.ReasonCode)
	}
	if len(out.Request.Scope.Paths) != 1 || out.Request.Scope.Paths[0] != "pkg/a.py" {
		t.Fatalf("failure scope paths = %+v", out.Request.Scope.Paths)
	}
	if !stringSliceContains(out.Result.EvidenceRefs, "plan-1.attempt-2.diff") {
		t.Fatalf("failure evidence refs not carried: %+v", out.Result.EvidenceRefs)
	}
	assertWorkerResultValid(t, out.Request, out.Result)
}

func assertWorkerResultValid(t *testing.T, req Request, result Result) {
	t.Helper()
	if errs := ValidateRequest(req); len(errs) != 0 {
		t.Fatalf("request should validate: %+v", errs)
	}
	if errs := ValidateResult(result); len(errs) != 0 {
		t.Fatalf("result should validate: %+v", errs)
	}
}
