package types

import (
	"testing"
	"time"
)

func TestImpactAnalysisResultFromObligationSetBuildsSurfacesEdgesAndTargets(t *testing.T) {
	created := time.Unix(10, 0)
	set := ImpactObligationSet{
		PlanID: "plan-impact",
		Obligations: []ImpactObligation{{
			Kind:          "changed_symbol",
			Relation:      "patch_hunk",
			Obligation:    "verify_changed_symbol",
			SubjectPath:   "pkg/a.py",
			SubjectSymbol: "pkg.target",
			Strength:      ImpactObligationStrengthPrecise,
			EvidenceRef:   "pkg/a.py:10",
		}, {
			Kind:        "dependent",
			Relation:    "reverse_import",
			Obligation:  "verify_dependent",
			SubjectPath: "pkg/a.py",
			RelatedPath: "pkg/caller.py",
			Strength:    ImpactObligationStrengthInferred,
			EvidenceRef: "pkg/caller.py->pkg/a.py",
		}, {
			Kind:              "behavior_contract",
			Relation:          "contract_ref",
			Obligation:        "verify_contract_ref",
			ContractRef:       "contract-1",
			VerificationProbe: "probe-1",
			Strength:          ImpactObligationStrengthDeclared,
			EvidenceRef:       "contract-1",
		}},
	}
	result := ImpactAnalysisResultFromObligationSet("plan-impact", "patch-effect:1", set, created)
	if result.ResultID != "impact-analysis:plan-impact:patch-effect:1" ||
		result.PlanID != "plan-impact" ||
		result.PatchEffectID != "patch-effect:1" ||
		result.CreatedAt != created {
		t.Fatalf("result identity wrong: %+v", result)
	}
	if !impactAnalysisHasSurface(result, "symbol", "pkg/a.py", "pkg.target") {
		t.Fatalf("changed symbol surface missing: %+v", result.ChangedSurfaces)
	}
	if !impactAnalysisHasEdge(result, "dependent", "reverse_import", "pkg/a.py", "pkg/caller.py") {
		t.Fatalf("dependent edge missing: %+v", result.Edges)
	}
	if len(result.VerificationTargets) < 3 || result.VerificationTargets[0].Kind != "behavior_contract" {
		t.Fatalf("verification targets not prioritized: %+v", result.VerificationTargets)
	}
	if result.VerificationTargets[0].CoverageStatus != "unverified" {
		t.Fatalf("default coverage should be unverified: %+v", result.VerificationTargets[0])
	}
}

func impactAnalysisHasSurface(result ImpactAnalysisResult, kind, path, symbol string) bool {
	for _, surface := range result.ChangedSurfaces {
		if surface.Kind == kind && surface.Path == path && surface.Symbol == symbol {
			return true
		}
	}
	return false
}

func impactAnalysisHasEdge(result ImpactAnalysisResult, kind, relation, from, to string) bool {
	for _, edge := range result.Edges {
		if edge.Kind == kind && edge.Relation == relation && edge.FromPath == from && edge.ToPath == to {
			return true
		}
	}
	return false
}
