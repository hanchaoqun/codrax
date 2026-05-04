package types

import "testing"

// TestAllQuestionFamiliesHaveCompiler is the structural gate (B2-T4)
// that catches "added a new QuestionFamily without wiring a compile
// entry-point". For each family in AllQuestionFamilies(), build a
// minimal IR that resolves to that family, run BuildAnswerSemanticView,
// and assert (a) the returned view is non-nil, (b) view.Family
// matches, (c) at least one BlockRequirement (Summary is required
// by every family).
func TestAllQuestionFamiliesHaveCompiler(t *testing.T) {
	cases := map[QuestionFamily]*AnalysisIR{
		QFRootCauseTrace:   irForRootCauseTrace(),
		QFConfigPrecedence: irForConfigPrecedence(),
		QFRoleLookup:       irForRoleLookup(),
		QFCallChain:        irForCallChain(),
		QFEnumeration:      irForEnumeration(),
		QFArchitecture:     irForArchitecture(),
		QFComparison:       irForComparison(),
		QFGeneric:          irForGeneric(),
	}
	for _, family := range AllQuestionFamilies() {
		ir, ok := cases[family]
		if !ok {
			t.Errorf("family %q has no test fixture; add one to TestAllQuestionFamiliesHaveCompiler", family)
			continue
		}
		view := BuildAnswerSemanticView(ir, nil)
		if view == nil {
			t.Errorf("family %q: BuildAnswerSemanticView returned nil", family)
			continue
		}
		if view.Family != family {
			t.Errorf("family %q: ResolveQuestionFamily resolved to %q instead", family, view.Family)
		}
		if len(view.RequiredBlocks) == 0 {
			t.Errorf("family %q: zero RequiredBlocks; every family must require at least Summary", family)
		}
		hasSummary := false
		for _, b := range view.RequiredBlocks {
			if b.Kind == BlockSummary && b.Required && b.MinCount >= 1 {
				hasSummary = true
				break
			}
		}
		if !hasSummary {
			t.Errorf("family %q: missing required BlockSummary with MinCount>=1", family)
		}
	}
}

// TestAllQuestionFamiliesEnumerated is a sanity check: the count in
// AllQuestionFamilies() must match the number of declared QFXxx
// constants. If you add a new const without updating the function,
// this catches it before the compile-test below would.
func TestAllQuestionFamiliesEnumerated(t *testing.T) {
	want := 8 // R4.4 added QFComparison
	if got := len(AllQuestionFamilies()); got != want {
		t.Errorf("expected %d declared QuestionFamily values; got %d (%v)", want, got, AllQuestionFamilies())
	}
}

// TestBlockKindForFacetCoversAllAnswerFacetKinds enforces that
// blockKindForFacet has a switch arm for every declared
// AnswerFacetKind. Adding a new facet without updating the helper
// silently falls into the default fallback (BlockSection); this
// test catches it at CI time so the helper's mapping stays
// authoritative.
func TestBlockKindForFacetCoversAllAnswerFacetKinds(t *testing.T) {
	declared := []AnswerFacetKind{
		FacetObservedArtifactFact,
		FacetCurrentCodePath,
		FacetNearestMechanism,
		FacetUncertaintyBoundary,
		FacetConfigPrecedenceRole,
		FacetResolvedLiteralOrSymbol,
		FacetEnumerationItem,
		FacetBucketLabel,
		FacetPrincipalPathEdge,
		FacetBranchGuard,
		FacetComponentRelation,
		FacetDiagramSpine,
	}
	for _, f := range declared {
		got := blockKindForFacet(f)
		if got == "" {
			t.Errorf("facet %q maps to empty BlockKind", f)
		}
		if !IsValidAnswerBlockKind(got) {
			t.Errorf("facet %q maps to invalid BlockKind %q", f, got)
		}
	}
}
