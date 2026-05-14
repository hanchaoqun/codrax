package types

import "testing"

// ── B1-T1 AnswerBlockKind 闭枚举 ───────────────────────────────────

func TestAllAnswerBlockKindsCovered(t *testing.T) {
	kinds := AllAnswerBlockKinds()
	if len(kinds) != 9 {
		t.Fatalf("expected 9 declared block kinds; got %d (%v)", len(kinds), kinds)
	}
	seen := make(map[AnswerBlockKind]bool, len(kinds))
	for _, k := range kinds {
		if k == "" {
			t.Errorf("empty AnswerBlockKind in AllAnswerBlockKinds()")
		}
		if seen[k] {
			t.Errorf("duplicate AnswerBlockKind %q in AllAnswerBlockKinds()", k)
		}
		seen[k] = true
	}
	// Spot-check a few canonical members.
	for _, want := range []AnswerBlockKind{BlockSummary, BlockDiagram, BlockCaveat, BlockOrderedList} {
		if !seen[want] {
			t.Errorf("AllAnswerBlockKinds() missing %q", want)
		}
	}
}

func TestIsValidAnswerBlockKind(t *testing.T) {
	for _, k := range AllAnswerBlockKinds() {
		if !IsValidAnswerBlockKind(k) {
			t.Errorf("declared kind %q not accepted by IsValidAnswerBlockKind", k)
		}
	}
	for _, bad := range []AnswerBlockKind{"", "shape", "list_of_symbols", "bogus"} {
		if IsValidAnswerBlockKind(bad) {
			t.Errorf("invalid kind %q accepted by IsValidAnswerBlockKind", bad)
		}
	}
}

// ── B1-T3 BuildAnswerSemanticView 骨架 ────────────────────────────

func TestBuildAnswerSemanticView_NilInputReturnsNil(t *testing.T) {
	if got := BuildAnswerSemanticView(nil, nil); got != nil {
		t.Errorf("nil ir should yield nil view; got %+v", got)
	}
	if got := BuildAnswerSemanticViewForAgentContext(nil); got != nil {
		t.Errorf("nil ac should yield nil view; got %+v", got)
	}
	if got := BuildAnswerSemanticViewForBusContext(nil); got != nil {
		t.Errorf("nil bus should yield nil view; got %+v", got)
	}
}

// AnswerShape constants retired in PR5 of the AnswerShape
// terminal-retirement migration. The
// "EveryShapeProducesNonNilView" loop test that lived here is
// gone with them — V2 view rendering keys off QuestionFamily
// (covered by the QuestionFamily helper tests in
// answer_semantic_view_helpers_test.go).

func TestBuildAnswerSemanticView_FacetCoverageAliasedFromPlan(t *testing.T) {
	fc := &FacetCoverageContract{Family: QFRoleLookup}
	plan := &AnswerSurfacePlan{
		FacetCoverage:      fc,
		SummarySurfaceMode: AnswerSummarySurfaceMinimalScalarRoleLocate,
	}
	ir := &AnalysisIR{
		RequestModel: RequestModel{Intent: IntentExplain},
	}
	view := BuildAnswerSemanticView(ir, plan)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.FacetCoverage != fc {
		t.Errorf("FacetCoverage not aliased from plan; got %p want %p", view.FacetCoverage, fc)
	}
	if view.SummaryMode != AnswerSummarySurfaceMinimalScalarRoleLocate {
		t.Errorf("SummaryMode not propagated; got %q", view.SummaryMode)
	}
}

func TestBuildAnswerSemanticView_ExactResolutionPropagated(t *testing.T) {
	er := &ExactResolutionContract{}
	ir := &AnalysisIR{
		RequestModel: RequestModel{Intent: IntentExplain},
		AnswerContract: AnswerContract{
			ExactResolution: er,
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil || view.ExactResolution != er {
		t.Errorf("ExactResolution not aliased; view=%+v", view)
	}
}

func TestBuildAnswerSemanticView_RequestedCandidateRolesPropagated(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentReturnValue,
			Predicates: SemanticPredicates{
				IsScalarAnswer: true,
			},
			AnswerRoleProfile: &AnswerRoleProfile{
				IsRoleBindingRequested: true,
				RequiredCandidateRoles: []AnswerCandidateRole{
					AnswerCandidateRoleBudgetCap,
					AnswerCandidateRoleBudgetCap,
					AnswerCandidateRoleAttemptCounter,
				},
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if got := view.RequiredCandidateRoles; len(got) != 2 ||
		got[0] != AnswerCandidateRoleBudgetCap ||
		got[1] != AnswerCandidateRoleAttemptCounter {
		t.Fatalf("required candidate roles not propagated/deduped: %+v", got)
	}
}

func TestBuildAnswerSemanticView_ErrorGranularityRequiresPrincipalDecision(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentReturnValue,
			Predicates: SemanticPredicates{
				IsScalarAnswer: true,
			},
			ErrorGranularityProfile: &ErrorGranularityProfile{
				IsGranularityQuestion: true,
				Confidence:            0.9,
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if view.ErrorGranularityProfile == nil || !view.ErrorGranularityProfile.Active() {
		t.Fatalf("error granularity profile not propagated: %+v", view.ErrorGranularityProfile)
	}
	for _, req := range view.RequiredBlocks {
		if req.Kind != BlockDecision {
			continue
		}
		if !req.Required || req.MinCount != 1 || req.MaxCount != 1 || req.SurfaceRoleHint != SurfacePrincipal {
			t.Fatalf("decision block requirement wrong: %+v", req)
		}
		return
	}
	t.Fatalf("missing required decision block: %+v", view.RequiredBlocks)
}
