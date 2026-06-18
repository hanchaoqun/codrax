package types

import "testing"

func TestNormalizePatchReviewRecordAddsCoverageSummary(t *testing.T) {
	review := NormalizePatchReviewRecord(PatchReviewRecord{
		Findings: []PatchReviewFinding{{
			Code:           "changed_symbol_without_probe_coverage",
			Severity:       PatchReviewSeverityWarning,
			Category:       PatchReviewCategorySemanticCoverage,
			ImpactKind:     PatchReviewImpactKindChangedSymbol,
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
		review.CoverageSummary.PrimaryUncoveredImpactKind != PatchReviewImpactKindChangedSymbol ||
		review.CoverageSummary.BlockReason != "patch_review_semantic_uncovered:changed_symbol_without_probe_coverage" {
		t.Fatalf("unexpected coverage summary: %+v", review.CoverageSummary)
	}
	if len(review.CoverageSummary.ImpactKindCoverage) != 1 ||
		review.CoverageSummary.ImpactKindCoverage[0].Kind != PatchReviewImpactKindChangedSymbol ||
		review.CoverageSummary.ImpactKindCoverage[0].Unverified != 1 {
		t.Fatalf("impact-kind coverage summary missing changed-symbol gap: %+v", review.CoverageSummary.ImpactKindCoverage)
	}
	if !PatchReviewHasUncoveredSemanticCoverage(&review) {
		t.Fatalf("uncovered semantic coverage helper returned false")
	}
}

func TestNormalizePatchReviewRecordSummaryBackfillsLegacyImpactKindFromCode(t *testing.T) {
	review := NormalizePatchReviewRecord(PatchReviewRecord{
		Findings: []PatchReviewFinding{{
			Code:           "python_nested_string_key_direct_access_added",
			Severity:       PatchReviewSeverityWarning,
			Category:       PatchReviewCategorySemanticCoverage,
			CoverageStatus: PatchReviewCoverageUnknown,
			Path:           "pkg/widget.py",
		}},
	})

	if review.CoverageSummary == nil {
		t.Fatalf("coverage summary missing: %+v", review)
	}
	if review.CoverageSummary.PrimaryUncoveredImpactKind != PatchReviewImpactKindEffectFollowup {
		t.Fatalf("legacy actual-diff code should summarize as effect follow-up: %+v", review.CoverageSummary)
	}
	if len(review.CoverageSummary.ImpactKindCoverage) != 1 ||
		review.CoverageSummary.ImpactKindCoverage[0].Kind != PatchReviewImpactKindEffectFollowup ||
		review.CoverageSummary.ImpactKindCoverage[0].Unknown != 1 {
		t.Fatalf("unexpected effect-followup coverage: %+v", review.CoverageSummary.ImpactKindCoverage)
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

func TestNormalizePatchReviewRecordNormalizesEvidenceProjection(t *testing.T) {
	review := NormalizePatchReviewRecord(PatchReviewRecord{
		Findings: []PatchReviewFinding{{
			Code:           " convention_surface_available ",
			Severity:       PatchReviewSeverityInfo,
			Category:       PatchReviewCategoryConvention,
			CoverageStatus: PatchReviewCoverageAdvisory,
			SourceStage:    " convention_graph ",
			LineStart:      24,
			LineEnd:        12,
			ContextSummary: " repository local relation ",
			EvidenceRef:    " evidence:axis ",
		}},
	})

	if len(review.Findings) != 1 {
		t.Fatalf("finding missing after normalize: %+v", review)
	}
	finding := review.Findings[0]
	if finding.Code != "convention_surface_available" ||
		finding.SourceStage != "convention_graph" ||
		finding.LineStart != 24 ||
		finding.LineEnd != 24 ||
		finding.ContextSummary != "repository local relation" ||
		finding.EvidenceRef != "evidence:axis" {
		t.Fatalf("typed evidence projection not normalized: %+v", finding)
	}
}
