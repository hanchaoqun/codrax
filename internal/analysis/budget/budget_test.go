package budget

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// projectOrientationRM returns a RequestModel that satisfies
// types.IsProjectOrientationQuestion: intent=explain + simple
// complexity + no entities + clean predicates.
func projectOrientationRM() types.RequestModel {
	return types.RequestModel{
		Intent:     types.IntentExplain,
		Complexity: types.ComplexitySimple,
	}
}

// modeRateRM returns the historical "moderate complexity, no
// orientation cues" RequestModel. PrimaryEntities is non-empty so
// the orientation predicate must reject it.
func moderateRM() types.RequestModel {
	return types.RequestModel{
		Intent:     types.IntentExplain,
		Complexity: types.ComplexityModerate,
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"explorer"},
		},
	}
}

// TestCompute_ProjectOrientation_ShrinksBase locks the 2026-04-29
// carve-out: when the request matches IsProjectOrientationQuestion
// the EvidenceBudget base should drop to {files: 8, iters: 6}
// regardless of complexity. With unit term/hyp/probe factors the
// returned MaxFiles == 8 and MaxReactIters == 6 exactly.
func TestCompute_ProjectOrientation_ShrinksBase(t *testing.T) {
	rm := projectOrientationRM()
	// Term=8 → factor=1.0; HypCount=3 → factor=1.0; PrescanHitRatio=1
	// → factor=1.0. mult == 1.0, so output equals base.
	sig := BudgetSignals{
		Complexity:      types.ComplexitySimple,
		TermCount:       8,
		HypothesisCount: 3,
		PrescanHitRatio: 1.0,
	}
	got := Compute(rm, sig)
	if got.MaxFiles != 8 {
		t.Errorf("MaxFiles = %d, want 8 (project-orientation base)", got.MaxFiles)
	}
	if got.MaxReactIters != 6 {
		t.Errorf("MaxReactIters = %d, want 6 (project-orientation base)", got.MaxReactIters)
	}
}

// TestCompute_NotOrientation_KeepsModerateBase locks the
// non-orientation path: anything that fails the orientation
// predicate (PrimaryEntities present, etc.) keeps the historical
// complexity-only base. With unit factors moderate complexity
// produces files=30, iters=16.
func TestCompute_NotOrientation_KeepsModerateBase(t *testing.T) {
	rm := moderateRM()
	sig := BudgetSignals{
		Complexity:      types.ComplexityModerate,
		TermCount:       8,
		HypothesisCount: 3,
		PrescanHitRatio: 1.0,
	}
	got := Compute(rm, sig)
	if got.MaxFiles != 30 {
		t.Errorf("MaxFiles = %d, want 30 (moderate base, no orientation)", got.MaxFiles)
	}
	if got.MaxReactIters != 16 {
		t.Errorf("MaxReactIters = %d, want 16 (moderate base, no orientation)", got.MaxReactIters)
	}
}

// TestCompute_OrientationOverridesComplexity proves the orientation
// short-circuit precedes the complexity switch — a moderate-rated
// orientation question (the analyzer rates the question simple but
// the BudgetSignals.Complexity caller passes moderate by mistake)
// still gets the orientation base because the predicate reads from
// the RequestModel itself.
func TestCompute_OrientationOverridesComplexity(t *testing.T) {
	rm := projectOrientationRM()
	sig := BudgetSignals{
		Complexity:      types.ComplexityModerate, // caller mismatch
		TermCount:       8,
		HypothesisCount: 3,
		PrescanHitRatio: 1.0,
	}
	got := Compute(rm, sig)
	if got.MaxFiles != 8 {
		t.Errorf("MaxFiles = %d, want 8 — orientation predicate must beat caller-passed complexity", got.MaxFiles)
	}
}
