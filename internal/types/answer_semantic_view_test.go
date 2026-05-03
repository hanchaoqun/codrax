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

func TestBuildAnswerSemanticView_EveryShapeProducesNonNilView(t *testing.T) {
	for _, shape := range []AnswerShape{
		ShapeListOfSymbols,
		ShapeStepList,
		ShapeValue,
		ShapeBoolean,
		ShapeConfigValue,
		ShapeExplanation,
	} {
		ir := &AnalysisIR{
			RequestModel: RequestModel{
				Intent:   IntentExplain,
				Scenario: ScenarioGeneric,
			},
			AnswerContract: AnswerContract{
				RequiredAnswerShape: shape,
			},
		}
		view := BuildAnswerSemanticView(ir, nil)
		if view == nil {
			t.Errorf("shape=%s: expected non-nil view; got nil", shape)
			continue
		}
		// B1 skeleton: family resolves, no blocks yet.
		if view.Family == "" {
			t.Errorf("shape=%s: Family empty (ResolveQuestionFamily failed to map)", shape)
		}
	}
}

func TestBuildAnswerSemanticView_FacetCoverageAliasedFromPlan(t *testing.T) {
	fc := &FacetCoverageContract{Family: QFRoleLookup}
	plan := &AnswerSurfacePlan{
		FacetCoverage:      fc,
		SummarySurfaceMode: AnswerSummarySurfaceMinimalScalarRoleLocate,
	}
	ir := &AnalysisIR{
		RequestModel:   RequestModel{Intent: IntentExplain},
		AnswerContract: AnswerContract{RequiredAnswerShape: ShapeValue},
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
			RequiredAnswerShape: ShapeValue,
			ExactResolution:     er,
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil || view.ExactResolution != er {
		t.Errorf("ExactResolution not aliased; view=%+v", view)
	}
}
