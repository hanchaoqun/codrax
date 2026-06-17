package types

import "testing"

func TestNormalizePatchReviewRecordAddsCoverageSummary(t *testing.T) {
	review := NormalizePatchReviewRecord(PatchReviewRecord{
		Findings: []PatchReviewFinding{{
			Code:           "changed_symbol_without_probe_coverage",
			Severity:       PatchReviewSeverityWarning,
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageUnverified,
			SubjectSymbol:  "pkg.Target",
		}, {
			Code:           "convention_surface_available",
			Severity:       PatchReviewSeverityInfo,
			Category:       PatchReviewCategoryConvention,
			CoverageStatus: PatchReviewCoverageAdvisory,
		}},
	})

	if review.CoverageSummary == nil {
		t.Fatalf("coverage summary missing: %+v", review)
	}
	if review.CoverageSummary.Verdict != PatchReviewCoverageVerdictUnverified ||
		!review.CoverageSummary.HasUncoveredSemantic ||
		review.CoverageSummary.SemanticFindings != 1 ||
		review.CoverageSummary.UnverifiedSemantic != 1 ||
		review.CoverageSummary.AdvisoryFindings != 1 ||
		review.CoverageSummary.BlockReason != "patch_review_semantic_uncovered:changed_symbol_without_probe_coverage" {
		t.Fatalf("unexpected coverage summary: %+v", review.CoverageSummary)
	}
	if !PatchReviewHasUncoveredSemanticCoverage(&review) {
		t.Fatalf("uncovered semantic coverage helper returned false")
	}
}

func TestNormalizePatchReviewRecordSummaryTreatsVerifiedSemanticAsVerified(t *testing.T) {
	review := NormalizePatchReviewRecord(PatchReviewRecord{
		Findings: []PatchReviewFinding{{
			Code:           "changed_symbol_without_probe_coverage",
			Severity:       PatchReviewSeverityWarning,
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageVerified,
			SubjectSymbol:  "pkg.Target",
		}},
	})

	if review.CoverageSummary == nil {
		t.Fatalf("coverage summary missing: %+v", review)
	}
	if review.CoverageSummary.Verdict != PatchReviewCoverageVerdictVerified ||
		review.CoverageSummary.HasUncoveredSemantic ||
		review.CoverageSummary.VerifiedSemantic != 1 {
		t.Fatalf("unexpected verified summary: %+v", review.CoverageSummary)
	}
	if PatchReviewHasUncoveredSemanticCoverage(&review) {
		t.Fatalf("verified semantic coverage should not be uncovered")
	}
}

func TestNormalizePatchReviewRecordSummaryTreatsErrorAsFailed(t *testing.T) {
	review := NormalizePatchReviewRecord(PatchReviewRecord{
		Findings: []PatchReviewFinding{{
			Code:     "patch_effect_path_outside_plan_scope",
			Severity: PatchReviewSeverityError,
			Category: PatchReviewCategoryScope,
			Path:     "pkg/outside.py",
		}},
	})

	if review.CoverageSummary == nil {
		t.Fatalf("coverage summary missing: %+v", review)
	}
	if !review.HardBlock ||
		review.CoverageSummary.Verdict != PatchReviewCoverageVerdictFailed ||
		review.CoverageSummary.ErrorFindings != 1 ||
		review.CoverageSummary.BlockReason != "patch_review_error:patch_effect_path_outside_plan_scope" {
		t.Fatalf("unexpected failed summary: %+v review=%+v", review.CoverageSummary, review)
	}
}

func TestNormalizePatchReviewRecordSummaryDoesNotTreatAdvisoryAsUncovered(t *testing.T) {
	review := NormalizePatchReviewRecord(PatchReviewRecord{
		Findings: []PatchReviewFinding{{
			Code:           "convention_surface_available",
			Severity:       PatchReviewSeverityInfo,
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageAdvisory,
			SubjectSymbol:  "pkg.Target",
		}},
	})

	if review.CoverageSummary == nil {
		t.Fatalf("coverage summary missing: %+v", review)
	}
	if review.CoverageSummary.Verdict != PatchReviewCoverageVerdictAdvisory ||
		review.CoverageSummary.HasUncoveredSemantic ||
		review.CoverageSummary.UnverifiedSemantic != 0 ||
		review.CoverageSummary.AdvisoryFindings != 1 {
		t.Fatalf("unexpected advisory summary: %+v", review.CoverageSummary)
	}
}
