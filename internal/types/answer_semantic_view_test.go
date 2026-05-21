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

func TestBuildAnswerSemanticView_RequiredMechanismAnchorsFromTypedMentionedLanes(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent:   IntentExplain,
			Scenario: ScenarioArchitectureExplain,
			AnalyzerHints: AnalyzerHints{
				MentionedEntities: []string{"runTaskGraph", "helperCandidate"},
				ExactTargets:      []string{"emit_evidence"},
			},
		},
		AnswerContract: AnswerContract{
			MustIncludeTerms: []ContractTerm{
				{Text: "runTaskGraph", Kind: ContractTermSymbol, Source: ContractTermSourceAnalyzerEntity},
				{Text: "emit_evidence", Kind: ContractTermToolName, Source: ContractTermSourceAnalyzerEntity},
				{Text: "derivedOnly", Kind: ContractTermSymbol, Source: ContractTermSourceAnalyzerEntity},
				{Text: "upstream evidence node", Kind: ContractTermUserPhrase},
			},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	got := view.RequiredMechanismAnchors
	if len(got) != 3 {
		t.Fatalf("required anchors len=%d want 3: %+v", len(got), got)
	}
	if got[0].Text != "runTaskGraph" || got[0].Kind != ContractTermSymbol {
		t.Fatalf("first anchor = %+v, want runTaskGraph symbol", got[0])
	}
	if got[1].Text != "emit_evidence" || got[1].Kind != ContractTermToolName {
		t.Fatalf("second anchor = %+v, want emit_evidence tool_name", got[1])
	}
	if got[2].Text != "helperCandidate" || got[2].Kind != ContractTermSymbol {
		t.Fatalf("third anchor = %+v, want helperCandidate symbol", got[2])
	}
}

func TestBuildAnswerSemanticView_RequiredMechanismAnchorsDisabledForScalar(t *testing.T) {
	ir := &AnalysisIR{
		RequestModel: RequestModel{
			Intent: IntentReturnValue,
			Predicates: SemanticPredicates{
				IsScalarAnswer: true,
			},
			AnalyzerHints: AnalyzerHints{
				MentionedEntities: []string{"runTaskGraph"},
			},
		},
		AnswerContract: AnswerContract{
			MustIncludeTerms: []ContractTerm{{Text: "runTaskGraph", Kind: ContractTermSymbol}},
		},
	}
	view := BuildAnswerSemanticView(ir, nil)
	if view == nil {
		t.Fatal("view nil")
	}
	if len(view.RequiredMechanismAnchors) != 0 {
		t.Fatalf("scalar lookup should not gain mechanism anchors: %+v", view.RequiredMechanismAnchors)
	}
}

func TestMissingRequiredMechanismAnchors_UsesStructuredFieldsOnly(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "runTaskGraph", Kind: ContractTermSymbol},
		{Text: "EdgeValidationFeedback", Kind: ContractTermSymbol},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{
		{
			ID:   "summary",
			Kind: BlockSummary,
			Text: "The prose mentions EdgeValidationFeedback, but prose is not the anchor carrier.",
		},
		{
			ID:    "anchors",
			Kind:  BlockOrderedList,
			Items: []AnswerBlockItem{{Label: "`runTaskGraph`"}},
		},
	}}
	missing := MissingRequiredMechanismAnchors(doc, required)
	if len(missing) != 1 || missing[0].Text != "EdgeValidationFeedback" {
		t.Fatalf("missing anchors = %+v, want EdgeValidationFeedback only", missing)
	}
	doc.Blocks[1].Items = append(doc.Blocks[1].Items, AnswerBlockItem{Label: "EdgeValidationFeedback()"})
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("structured item labels should satisfy anchors, missing %+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_StructuredQualifiedLabelsSatisfyParts(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "StageOutput", Kind: ContractTermSymbol},
		{Text: "AnalysisIR", Kind: ContractTermSymbol},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:   "mechanism",
		Kind: BlockSection,
		Items: []AnswerBlockItem{{
			Label: "StageOutput.AnalysisIR",
			Text:  "AnalysisIR is the analyzer result field on StageOutput.",
		}},
	}}}
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("qualified structured labels should satisfy owner/member anchors, missing %+v", missing)
	}
}

func TestMissingRequiredMechanismAnchors_StructuredIdentifierVariantSatisfiesToolName(t *testing.T) {
	required := []AnswerRequiredAnchor{
		{Text: "emit_analysis", Kind: ContractTermToolName},
	}
	doc := &AnswerDocumentV2{Blocks: []AnswerBlock{{
		ID:   "mechanism",
		Kind: BlockSection,
		Items: []AnswerBlockItem{{
			Label: "EmitAnalysis",
			Text:  "tool carrier type",
		}},
	}}}
	if missing := MissingRequiredMechanismAnchors(doc, required); len(missing) != 0 {
		t.Fatalf("structured code identifier variants should satisfy tool-name anchors, missing %+v", missing)
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
