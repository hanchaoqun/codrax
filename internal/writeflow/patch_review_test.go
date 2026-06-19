package writeflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReviewAppliedPatchScopePassesActiveSliceScope(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py", "b.py"},
		AppliedPaths: []string{"a.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{
		ID:    "slice-1",
		Paths: []string{"a.py"},
	})
	if review.HardBlock || review.Status != "passed" {
		t.Fatalf("in-scope patch should pass, got %+v", review)
	}
	if len(review.AppliedPaths) != 1 || review.AppliedPaths[0] != "a.py" {
		t.Fatalf("applied paths not normalized: %+v", review.AppliedPaths)
	}
}

func TestReviewAppliedPatchScopeIgnoresCumulativeDeclaredPathOutsideActiveSlice(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py", "b.py"},
		AppliedPaths: []string{"b.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{
		ID:    "slice-1",
		Paths: []string{"a.py"},
	})
	if review.HardBlock || review.Status != "passed" {
		t.Fatalf("cumulative declared path outside active slice should not hard block without patch effect, got %+v", review)
	}
	if len(review.AppliedPaths) != 0 {
		t.Fatalf("declared applied paths should be filtered to active slice, got %+v", review.AppliedPaths)
	}
}

func TestReviewAppliedPatchScopeBlocksOutsidePlanTarget(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py"},
		AppliedPaths: []string{"c.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if !review.HardBlock {
		t.Fatalf("outside-plan patch should hard block, got %+v", review)
	}
	if !patchReviewHasFinding(review, "applied_path_outside_plan_scope") {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
}

func TestReviewAppliedPatchScopeBlocksActualDiffOutsidePlanTarget(t *testing.T) {
	plan := &types.ChangePlan{
		ID:           "plan-1",
		Status:       types.PlanStatusAppliedPendingVerify,
		TargetPaths:  []string{"a.py"},
		AppliedPaths: []string{"a.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID:        "patch-effect:plan-1:slice-1:abcdef123456",
			DiffFingerprint: "abcdef1234567890",
			Files: []types.PatchEffectFile{{
				Path:   "c.py",
				Status: "modified",
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if !review.HardBlock {
		t.Fatalf("actual diff outside plan should hard block, got %+v", review)
	}
	if review.PatchEffectID == "" || review.DiffFingerprint == "" {
		t.Fatalf("patch effect metadata missing from review: %+v", review)
	}
	if !patchReviewHasFinding(review, "patch_effect_path_outside_plan_scope") {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
	if !patchReviewPathPresent(review.AppliedPaths, "a.py") || !patchReviewPathPresent(review.AppliedPaths, "c.py") {
		t.Fatalf("applied paths should merge declared and actual diff paths: %+v", review.AppliedPaths)
	}
}

func TestReviewAppliedPatchScopeBlocksActualDiffOutsideActiveSlice(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"a.py", "b.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "b.py",
				Status: "modified",
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{
		ID:    "slice-1",
		Paths: []string{"a.py"},
	})
	if !review.HardBlock {
		t.Fatalf("actual diff outside active slice should hard block, got %+v", review)
	}
	if !patchReviewHasFinding(review, "patch_effect_path_outside_active_slice") {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
}

func TestReviewAppliedPatchScopeUsesPatchEffectWhenDeclaredAppliedPathsMissing(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"a.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "a.py",
				Status: "modified",
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if review.HardBlock || review.Status != "passed" {
		t.Fatalf("in-scope actual diff should pass, got %+v", review)
	}
	if patchReviewHasFinding(review, "applied_paths_missing") {
		t.Fatalf("actual diff paths should satisfy applied-path evidence: %+v", review.Findings)
	}
}

func TestReviewAppliedPatchScopeCarriesSoftPatchEffectEvents(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"a.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "a.py",
				Status: "modified",
				Events: []types.PatchEffectEvent{{
					Code:     "control_flow_guard_touched",
					Severity: "warning",
					Path:     "a.py",
				}},
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("soft patch effect event should not hard block: %+v", review)
	}
	if !patchReviewHasFinding(review, "control_flow_guard_touched") {
		t.Fatalf("soft patch effect finding missing: %+v", review.Findings)
	}
	finding := patchReviewFindingByCode(review, "control_flow_guard_touched")
	if finding.Category != types.PatchReviewCategorySemanticCoverage {
		t.Fatalf("soft patch effect finding should carry semantic category: %+v", finding)
	}
}

func TestReviewAppliedPatchSemanticAddsImpactCoverageFindings(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"pkg/axis.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "pkg/axis.py",
				Status: "modified",
			}},
		},
		ImpactObligations: &types.ImpactObligationSet{
			PlanID: "plan-1",
			Obligations: []types.ImpactObligation{{
				Kind:          "changed_symbol",
				Relation:      "patch_hunk",
				Obligation:    "verify_changed_symbol",
				SubjectPath:   "pkg/axis.py",
				SubjectSymbol: "Axis.convert",
				Strength:      types.ImpactObligationStrengthPrecise,
				EvidenceRef:   "pkg/axis.py:12",
			}, {
				Kind:        "dependent",
				Relation:    "reverse_import",
				Obligation:  "verify_dependent",
				SubjectPath: "pkg/axis.py",
				RelatedPath: "pkg/caller.py",
				Strength:    types.ImpactObligationStrengthInferred,
				EvidenceRef: "pkg/caller.py->pkg/axis.py",
			}},
		},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("semantic coverage findings should not hard block: %+v", review)
	}
	symbol := patchReviewFindingByCode(review, "changed_symbol_without_probe_coverage")
	if symbol.Category != types.PatchReviewCategorySemanticCoverage ||
		symbol.CoverageStatus != types.PatchReviewCoverageUnverified ||
		symbol.ImpactKind != types.PatchReviewImpactKindChangedSymbol ||
		symbol.SubjectSymbol != "Axis.convert" ||
		symbol.Strength != string(types.ImpactObligationStrengthPrecise) {
		t.Fatalf("changed-symbol semantic finding not typed: %+v", symbol)
	}
	dependent := patchReviewFindingByCode(review, "dependent_surface_without_verify_coverage")
	if dependent.Relation != "reverse_import" ||
		dependent.RelatedPath != "pkg/caller.py" ||
		dependent.ImpactKind != types.PatchReviewImpactKindDependent {
		t.Fatalf("dependent semantic finding not typed: %+v", dependent)
	}
}

func TestReviewAppliedPatchSemanticAddsConventionFindingAsAdvisory(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"pkg/axis.py"},
		PatchEffect: &types.PatchEffectRecord{
			RecordID: "patch-effect:plan-1:slice-1:abcdef123456",
			Files: []types.PatchEffectFile{{
				Path:   "pkg/axis.py",
				Status: "modified",
			}},
		},
	}
	review := ReviewAppliedPatchSemantic(SemanticPatchReviewInput{
		Plan: plan,
		ConventionGraph: &types.ConventionGraph{
			Nodes: []types.ConventionNode{{
				Category:      types.ConventionCategoryMechanism,
				Summary:       "axis converters validate input before conversion",
				Source:        "pkg/axis.py",
				LineStart:     12,
				LineEnd:       18,
				EvidenceRefID: "evidence:axis",
				SourceStage:   "convention_graph",
				Strength:      "repo_local",
			}},
		},
	})
	if review.HardBlock {
		t.Fatalf("convention finding must stay advisory: %+v", review)
	}
	finding := patchReviewFindingByCode(review, "convention_surface_available")
	if finding.Category != types.PatchReviewCategoryConvention ||
		finding.CoverageStatus != types.PatchReviewCoverageAdvisory ||
		finding.Relation != string(types.ConventionCategoryMechanism) {
		t.Fatalf("convention finding not typed advisory: %+v", finding)
	}
	if finding.ContextSummary != "axis converters validate input before conversion" ||
		finding.SourceStage != "convention_graph" ||
		finding.LineStart != 12 ||
		finding.LineEnd != 18 ||
		finding.EvidenceRef != "evidence:axis" {
		t.Fatalf("convention finding lost typed evidence: %+v", finding)
	}
}

func TestReviewAppliedPatchScopeWarnsWhenAppliedPathsMissing(t *testing.T) {
	plan := &types.ChangePlan{
		ID:          "plan-1",
		Status:      types.PlanStatusAppliedPendingVerify,
		TargetPaths: []string{"a.py"},
	}
	review := ReviewAppliedPatchScope(plan, types.ChangePlanSlice{})
	if review.HardBlock {
		t.Fatalf("missing applied_paths should warn, not block: %+v", review)
	}
	if len(review.Findings) != 1 || review.Findings[0].Code != "applied_paths_missing" {
		t.Fatalf("unexpected findings: %+v", review.Findings)
	}
}

func patchReviewPathPresent(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func patchReviewHasFinding(review types.PatchReviewRecord, code string) bool {
	return patchReviewFindingByCode(review, code).Code != ""
}

func patchReviewFindingByCode(review types.PatchReviewRecord, code string) types.PatchReviewFinding {
	for _, finding := range review.Findings {
		if finding.Code == code {
			return finding
		}
	}
	return types.PatchReviewFinding{}
}
